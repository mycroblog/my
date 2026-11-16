// SPDX-License-Identifier: AGPL-3.0-only

// Web Push in the E2E tests (ADR-0015 §10): the browser's push subscription comes from the fake push service that the
// demo server starts beside it (`my dev push-service`, src/server/demo.ts), and the messages the fake received and
// decrypted go to the page's real Service Worker over the Chrome DevTools Protocol.
//
// - `installPushShim` grants the notifications and replaces PushManager.prototype.subscribe, getSubscription and
//   permissionState with an init script: the page asks this process for a subscription through a binding
//   (exposeBinding), and this process asks the fake. The page never fetches the fake itself, so the app's CSP
//   (`connect-src 'self'`) stays as it is, and the app's code does not change. A headless browser has no push service.
// - `deliverPush` hands a plaintext to the Service Worker with ServiceWorker.deliverPushMessage of a page's
//   CDPSession, which fires the worker's `push` event with it (Chromium only).
// - `shownNotifications` reads what the worker shows (registration.getNotifications()). No protocol clicks a
//   notification (ADR-0015 §12): where a click leads is the address the notification keeps, which this reads.
//
// The default headless shell of Chromium reports the notifications' permission as denied whatever is granted, so a
// spec that shows notifications runs in the new headless mode (`channel: "chromium"`).

import { readFileSync } from "node:fs";
import { request as httpsRequest } from "node:https";
import type { BrowserContext, CDPSession, Page } from "@playwright/test";

/** A subscription that the fake issued, as PushSubscription.toJSON() shows it. */
type FakeSubscription = { endpoint: string; expirationTime: number | null; keys: { p256dh: string; auth: string } };

/** A message that the fake received (internal/adapter/pushservice/fake), accepted or refused. */
type FakeMessage = { status: number; error?: string; plaintext?: string; subject?: string };

/** The document the server encrypts for one notification (internal/app/notification PushPayload). */
export type PushPayload = { notification_id: string; type: string; title: string; lang: string; path: string };

/** The fake push service of the demo server: its origin and the CA of its certificate (specs/global-setup.ts). */
function pushService(): { origin: string; ca: Buffer } {
  const origin = process.env["E2E_PUSH_ORIGIN"];
  const ca = process.env["E2E_PUSH_CA"];
  if (origin === undefined || ca === undefined)
    throw new Error("e2e: no fake push service; run scripts/task.sh web:e2e");
  return { origin, ca: readFileSync(ca) };
}

/** Calls the fake's API at `path` over HTTPS, trusting the development CA; its status and body. */
function callFake(method: "GET" | "POST", path: string, body?: unknown): Promise<{ status: number; text: string }> {
  const { origin, ca } = pushService();
  return new Promise((resolve, reject) => {
    const req = httpsRequest(
      new URL(path, origin),
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

/** The payloads of the messages the fake accepted and decrypted for the subscription at `endpoint`, oldest first. */
export async function receivedPushes(endpoint: string): Promise<{ payload: PushPayload; plaintext: string }[]> {
  const { status, text } = await callFake("GET", `/subscriptions/${subscriptionID(endpoint)}/messages`);
  if (status !== 200) throw new Error(`the fake push service answered ${String(status)}: ${text}`);
  return (JSON.parse(text) as { messages: FakeMessage[] }).messages.flatMap((m) =>
    m.status === 201 && m.plaintext !== undefined
      ? [{ payload: JSON.parse(m.plaintext) as PushPayload, plaintext: m.plaintext }]
      : [],
  );
}

/**
 * The init script that replaces PushManager.prototype.subscribe, getSubscription and permissionState with the bindings
 * `__e2ePushSubscribe`, `__e2ePushGetSubscription` and `__e2ePushUnsubscribe`. It is self-contained: its source runs
 * as it is in the page.
 */
function pushManagerShim(): void {
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
}

/** The browser's side of Web Push in one context: the subscription the page made, kept in this process. */
export type PushShim = { subscription: () => FakeSubscription | undefined };

/**
 * Grants the notifications to `origin` and installs the PushManager shim in every page of `context`, which subscribes
 * with the fake push service. Call it before the context's first page opens.
 */
export async function installPushShim(context: BrowserContext, origin: string): Promise<PushShim> {
  await context.grantPermissions(["notifications"], { origin });
  let current: { subscription: FakeSubscription; applicationServerKey: string } | undefined;
  await context.exposeBinding("__e2ePushSubscribe", async (_source, applicationServerKey: string) => {
    const { status, text } = await callFake("POST", "/subscriptions", { applicationServerKey });
    if (status !== 201) throw new Error(`the fake push service answered ${String(status)}: ${text}`);
    current = { subscription: JSON.parse(text) as FakeSubscription, applicationServerKey };
    return current;
  });
  await context.exposeBinding("__e2ePushGetSubscription", () => current ?? null);
  await context.exposeBinding("__e2ePushUnsubscribe", () => {
    current = undefined;
    return true;
  });
  await context.addInitScript(pushManagerShim);
  return { subscription: () => current?.subscription };
}

/** Hands `data` to the Service Worker of `origin` as a push message, through a CDPSession of `page` (Chromium only). */
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

/** The notifications the Service Worker of `page`'s origin shows: their titles, and the address a click opens. */
export function shownNotifications(page: Page): Promise<{ title: string; url: string }[]> {
  return page.evaluate(async () => {
    const registration = await navigator.serviceWorker.ready;
    return (await registration.getNotifications()).map((n) => ({
      title: n.title,
      url: (n.data as { url?: string } | null)?.url ?? "",
    }));
  });
}
