// SPDX-License-Identifier: AGPL-3.0-only
import { useAnnouncer } from "@my/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import { instanceQueryOptions } from "../api/me";
import {
  cachePushSubscription,
  deletePushSubscription,
  type NotificationType,
  type PushSubscriptionInfo,
  pushSubscriptionQueryOptions,
  savePushSubscription,
} from "../api/push";

/** What the notification settings show of Web Push on this device. */
export type PushNotifications =
  /** The server sends no Web Push (the instance has no `push_server_key`), or that or the subscription is not known yet. */
  | { status: "hidden" }
  /** This browser has no Push API (or no Service Worker), so it cannot receive Web Push. */
  | { status: "unsupported" }
  /** The user blocked notifications for this site in the browser. */
  | { status: "denied" }
  | {
      status: "available";
      /** Whether this signed-in app has a subscription: the value being saved while `isPending`. */
      enabled: boolean;
      /** The types pushed: the value being saved while `isPending`. */
      alerts: readonly NotificationType[];
      setEnabled: (enabled: boolean) => void;
      /** Pushes `types` (one row of the notification settings) or stops pushing them. */
      setTypes: (types: readonly NotificationType[], selected: boolean) => void;
      isPending: boolean;
      /** Why the last change was not saved, localized; undefined when it was. */
      error: string | undefined;
    };

/** The parts of the browser that Web Push uses: the Push API and the Notifications API. */
function pushBrowser(): { container: ServiceWorkerContainer; notifications: typeof Notification } | null {
  if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return null;
  if (typeof PushManager === "undefined" || typeof Notification === "undefined") return null;
  return { container: navigator.serviceWorker, notifications: Notification };
}

/** Decodes unpadded base64url, the encoding of the server's key and the subscription's keys. */
function fromBase64Url(text: string): Uint8Array<ArrayBuffer> {
  const binary = atob(text.replaceAll("-", "+").replaceAll("_", "/"));
  return Uint8Array.from(binary, (c) => c.charCodeAt(0));
}

function sameBytes(a: ArrayBuffer | null, b: Uint8Array): boolean {
  if (a === null || a.byteLength !== b.byteLength) return false;
  const view = new Uint8Array(a);
  return view.every((byte, i) => byte === b[i]);
}

/** Thrown when the user does not allow notifications when asked. */
class PermissionRefused extends Error {}

/**
 * Returns the browser's subscription for the server's key, subscribing when there is none or when it was made
 * with another key (https://developer.mozilla.org/docs/Web/API/PushManager/subscribe). Every message is shown
 * as a notification (`userVisibleOnly`), as the Service Worker does.
 */
async function browserSubscription(container: ServiceWorkerContainer, serverKey: string): Promise<PushSubscription> {
  const registration = await container.ready;
  const key = fromBase64Url(serverKey);
  const existing = await registration.pushManager.getSubscription();
  if (existing !== null) {
    if (sameBytes(existing.options.applicationServerKey, key)) return existing;
    await existing.unsubscribe();
  }
  return registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
}

/** The request that registers `subscription` for `alerts`. */
function subscriptionRequest(subscription: PushSubscription, alerts: readonly NotificationType[]) {
  const json = subscription.toJSON();
  return {
    endpoint: subscription.endpoint,
    keys: { p256dh: json.keys?.["p256dh"] ?? "", auth: json.keys?.["auth"] ?? "" },
    alerts: [...alerts],
  };
}

/** A change of Web Push: turning it on with the types, changing the types, or turning it off. */
type PushChange = { enabled: true; alerts: readonly NotificationType[]; ask: boolean } | { enabled: false };

/**
 * Returns whether and how the notification settings offer Web Push on this device (FR-206, ADR-0109 §1). Turning it on
 * asks the browser for the permission to notify, subscribes with the server's key and registers the subscription for
 * this signed-in app with every type of `offered` (the rows of the notification settings); changing a row registers it
 * again; turning it off deletes the server's subscription and the browser's.
 */
export function usePushNotifications(offered: readonly NotificationType[]): PushNotifications {
  const api = useApi();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const t = useT();
  const browser = pushBrowser();
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const serverKey = instance.data?.push_server_key;
  const subscription = useQuery({
    ...pushSubscriptionQueryOptions(api),
    enabled: serverKey !== undefined && browser !== null,
  });
  const mutation = useMutation({
    mutationKey: ["put", "/push/subscription"] as const,
    mutationFn: async (change: PushChange): Promise<PushSubscriptionInfo | null> => {
      if (browser === null || serverKey === undefined) return null;
      if (!change.enabled) {
        await deletePushSubscription(api);
        const registration = await browser.container.ready;
        await (await registration.pushManager.getSubscription())?.unsubscribe();
        return null;
      }
      if (change.ask && (await browser.notifications.requestPermission()) !== "granted") throw new PermissionRefused();
      const pushSubscription = await browserSubscription(browser.container, serverKey);
      return savePushSubscription(api, subscriptionRequest(pushSubscription, change.alerts));
    },
    onSuccess: (saved) => {
      cachePushSubscription(api, queryClient, saved);
      announce(t("client:settings.saved"));
    },
  });

  if (serverKey === undefined) return { status: "hidden" };
  if (browser === null) return { status: "unsupported" };
  if (browser.notifications.permission === "denied") return { status: "denied" };
  // The section waits for the subscription of the grant, so that its switch never shows off before it is known.
  if (subscription.isPending) return { status: "hidden" };

  const saved = subscription.data ?? null;
  const pending = mutation.isPending ? mutation.variables : undefined;
  const enabled = pending?.enabled ?? saved !== null;
  const alerts = pending?.enabled === true ? pending.alerts : (saved?.alerts ?? offered);
  let error: string | undefined;
  if (subscription.isError) error = errorMessage(subscription.error);
  if (mutation.isError) {
    const failure = mutation.error;
    if (failure instanceof PermissionRefused) error = t("client:settings.push.not_allowed");
    else if (failure instanceof ApiError) error = errorMessage(failure);
    else error = t("client:settings.push.failed");
  }
  return {
    status: "available",
    enabled,
    alerts,
    setEnabled: (on) => {
      mutation.mutate(on ? { enabled: true, alerts: offered, ask: true } : { enabled: false });
    },
    setTypes: (types, selected) => {
      const next = selected ? [...new Set([...alerts, ...types])] : alerts.filter((type) => !types.includes(type));
      mutation.mutate({ enabled: true, alerts: next, ask: false });
    },
    isPending: mutation.isPending,
    error,
  };
}
