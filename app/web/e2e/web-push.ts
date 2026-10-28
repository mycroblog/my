// SPDX-License-Identifier: AGPL-3.0-only
// Web Push in the E2E tests of the demo server (ADR-0118 §2): the browser's push subscription comes from the fake push
// service that e2e/server.ts starts beside the demo server (`my dev push-service`), and the messages that the fake
// received and decrypted are handed to the page's real Service Worker over the Chrome DevTools Protocol.
//
// - `installPushShim` replaces PushManager.prototype.subscribe, getSubscription, and permissionState with an init
//   script. The page asks this process for a subscription through a binding (exposeBinding), and this process asks the
//   fake: the page never fetches the fake itself, so the app's CSP (`connect-src 'self'`) stays as it is, and the
//   app's code is not changed. A headless browser has no push service of its own to subscribe with.
// - `deliverPush` sends a plaintext to the Service Worker with ServiceWorker.deliverPushMessage of a page-target
//   CDPSession, which fires the worker's `push` event with that data (Chromium only).
//
// Checked in the Chromium of the pinned Playwright image (1.64.1, Chromium 156): the ServiceWorker domain of a page's
// CDPSession reports the registration, deliverPushMessage fires the worker's `push` handler, and showNotification /
// registration.getNotifications() work in the new headless mode (`channel: "chromium"`). The default headless shell
// (chromium-headless-shell) runs the `push` handler too, but reports Notification.permission "denied" whatever
// grantPermissions says, so showNotification fails there: the specs that show notifications use the new headless mode.
import { request as httpsRequest } from "node:https";
import { readFileSync } from "node:fs";
import type { BrowserContext, CDPSession, Page } from "@playwright/test";
import { mailpit, pushService } from "./server-cli.ts";

/** A subscription that the fake issued, as PushSubscription.toJSON() shows it. */
type FakeSubscription = {
  endpoint: string;
  expirationTime: number | null;
  keys: { p256dh: string; auth: string };
};

/** A push message that the fake received, accepted or refused (internal/adapter/pushservice/fake). */
export type FakeMessage = {
  received_at: string;
  status: number;
  error?: string;
  /** The decrypted payload of an accepted message. */
  plaintext?: string;
  ttl: number;
  urgency?: string;
  topic?: string;
  /** The sub claim of the verified VAPID token. */
  subject?: string;
};

/** The document that the server encrypts for one notification (internal/app/notification PushPayload). */
export type PushPayload = { notification_id: string; type: string; title: string; lang: string; path: string };

/** The development CA that signs the fake's certificate (scripts/mailpit-cert.sh). */
let caCache: Buffer | undefined;

/** Calls the fake's HTTP API at `path` over HTTPS, trusting the development CA; returns the status and the body. */
function callFake(method: "GET" | "POST", path: string, body?: unknown): Promise<{ status: number; text: string }> {
  caCache ??= readFileSync(mailpit.ca);
  const ca = caCache;
  return new Promise((resolve, reject) => {
    const req = httpsRequest(
      new URL(path, pushService.origin),
      { method, ca, headers: body === undefined ? {} : { "Content-Type": "application/json" } },
      (res) => {
        const chunks: Buffer[] = [];
        res.on("data", (chunk: Buffer) => chunks.push(chunk));
        res.on("end", () => {
          resolve({ status: res.statusCode ?? 0, text: Buffer.concat(chunks).toString("utf8") });
        });
        res.on("error", reject);
      },
    );
    req.on("error", reject);
    req.end(body === undefined ? undefined : JSON.stringify(body));
  });
}

/** The fake's ID of a subscription: the last segment of its endpoint (`<origin>/push/<id>`). */
function subscriptionID(endpoint: string): string {
  const id = /\/push\/([^/]+)$/.exec(new URL(endpoint).pathname)?.[1];
  if (id === undefined) throw new Error(`not an endpoint of the fake push service: ${endpoint}`);
  return id;
}

/** Asks the fake for a subscription for the application server key `applicationServerKey` (base64url). */
async function subscribe(applicationServerKey: string): Promise<FakeSubscription> {
  const { status, text } = await callFake("POST", "/subscriptions", { applicationServerKey });
  if (status !== 201) throw new Error(`the fake push service answered ${String(status)}: ${text}`);
  return JSON.parse(text) as FakeSubscription;
}

/** The messages that the fake received for the subscription at `endpoint`, oldest first. */
export async function fakeMessages(endpoint: string): Promise<FakeMessage[]> {
  const { status, text } = await callFake("GET", `/subscriptions/${subscriptionID(endpoint)}/messages`);
  if (status !== 200) throw new Error(`the fake push service answered ${String(status)}: ${text}`);
  return (JSON.parse(text) as { messages: FakeMessage[] }).messages;
}

/** Expires the subscription at `endpoint`: the fake answers 410 to every later message to it. */
export async function expireSubscription(endpoint: string): Promise<void> {
  const { status, text } = await callFake("POST", `/subscriptions/${subscriptionID(endpoint)}/expire`);
  if (status !== 204) throw new Error(`the fake push service answered ${String(status)}: ${text}`);
}

/** The payloads of the messages that the fake accepted and decrypted. */
export function payloadsOf(messages: readonly FakeMessage[]): PushPayload[] {
  return messages.flatMap((m) =>
    m.status === 201 && m.plaintext !== undefined ? [JSON.parse(m.plaintext) as PushPayload] : [],
  );
}

/** The browser's side of Web Push in one context: its subscription, which the shim keeps in this process. */
export type PushShim = {
  /** The subscription that the page subscribed, if any and not unsubscribed. */
  subscription: () => FakeSubscription | undefined;
};

/**
 * Grants the notification permission to `origin` and installs the PushManager shim in every page of `context`, which
 * subscribes with the fake push service (see the top of this file). Call it before the context's first page opens.
 */
export async function installPushShim(context: BrowserContext, origin: string): Promise<PushShim> {
  await context.grantPermissions(["notifications"], { origin });
  // The subscription and the application server key it was made with, kept here so that a reload keeps them.
  let current: { subscription: FakeSubscription; applicationServerKey: string } | undefined;
  await context.exposeBinding("__e2ePushSubscribe", async (_source, applicationServerKey: string) => {
    current = { subscription: await subscribe(applicationServerKey), applicationServerKey };
    return current;
  });
  await context.exposeBinding("__e2ePushGetSubscription", () => current ?? null);
  await context.exposeBinding("__e2ePushUnsubscribe", () => {
    current = undefined;
    return true;
  });
  await context.addInitScript(() => {
    type Issued = { endpoint: string; expirationTime: number | null; keys: { p256dh: string; auth: string } };
    type Current = { subscription: Issued; applicationServerKey: string };
    type Bindings = {
      __e2ePushSubscribe: (key: string) => Promise<Current>;
      __e2ePushGetSubscription: () => Promise<Current | null>;
      __e2ePushUnsubscribe: () => Promise<boolean>;
    };
    const bindings = window as unknown as Window & Bindings;
    if (typeof PushManager === "undefined") return;
    const toBase64Url = (bytes: Uint8Array) =>
      btoa(String.fromCharCode(...bytes))
        .replaceAll("+", "-")
        .replaceAll("/", "_")
        .replace(/=+$/, "");
    const fromBase64Url = (text: string) =>
      Uint8Array.from(atob(text.replaceAll("-", "+").replaceAll("_", "/")), (c) => c.charCodeAt(0)).buffer;
    const keyText = (key: BufferSource | string | null | undefined): string => {
      if (key === null || key === undefined) throw new TypeError("applicationServerKey is required");
      if (typeof key === "string") return key;
      const view =
        key instanceof ArrayBuffer ? new Uint8Array(key) : new Uint8Array(key.buffer, key.byteOffset, key.byteLength);
      return toBase64Url(view);
    };
    const shaped = (issued: Issued, applicationServerKey: string) => ({
      endpoint: issued.endpoint,
      expirationTime: issued.expirationTime,
      options: { userVisibleOnly: true, applicationServerKey: fromBase64Url(applicationServerKey) },
      getKey: (name: string) => {
        if (name === "p256dh") return fromBase64Url(issued.keys.p256dh);
        if (name === "auth") return fromBase64Url(issued.keys.auth);
        return null;
      },
      toJSON: () => ({ endpoint: issued.endpoint, expirationTime: issued.expirationTime, keys: { ...issued.keys } }),
      unsubscribe: () => bindings.__e2ePushUnsubscribe(),
    });
    PushManager.prototype.subscribe = async function subscribe(options?: PushSubscriptionOptionsInit) {
      const key = keyText(options?.applicationServerKey);
      const { subscription } = await bindings.__e2ePushSubscribe(key);
      return shaped(subscription, key);
    };
    PushManager.prototype.getSubscription = async function getSubscription() {
      const current = await bindings.__e2ePushGetSubscription();
      if (current === null) return null;
      return shaped(current.subscription, current.applicationServerKey);
    };
    PushManager.prototype.permissionState = function permissionState() {
      return Promise.resolve("granted" as PermissionState);
    };
  });
  return { subscription: () => current?.subscription };
}

/**
 * Hands `data` to the Service Worker of `origin` as a push message, through a CDPSession of `page` (Chromium only):
 * ServiceWorker.enable reports the registrations, and deliverPushMessage fires the worker's `push` event.
 */
export async function deliverPush(page: Page, origin: string, data: string): Promise<void> {
  const cdp: CDPSession = await page.context().newCDPSession(page);
  try {
    const scope = `${origin}/`;
    const registration = new Promise<string>((resolve) => {
      cdp.on("ServiceWorker.workerRegistrationUpdated", ({ registrations }) => {
        const found = registrations.find((r) => r.scopeURL === scope && !r.isDeleted);
        if (found !== undefined) resolve(found.registrationId);
      });
    });
    await cdp.send("ServiceWorker.enable");
    const registrationId = await registration;
    await cdp.send("ServiceWorker.deliverPushMessage", { origin, registrationId, data });
  } finally {
    await cdp.detach();
  }
}

/** The notifications that the Service Worker of `page`'s origin shows: their titles and tags. */
export function shownNotifications(page: Page): Promise<{ title: string; tag: string }[]> {
  return page.evaluate(async () => {
    const registration = await navigator.serviceWorker.ready;
    return (await registration.getNotifications()).map((n) => ({ title: n.title, tag: n.tag }));
  });
}
