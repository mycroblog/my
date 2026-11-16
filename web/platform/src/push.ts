// SPDX-License-Identifier: AGPL-3.0-only

// Web Push, the notifications' permission and the app's badge (FR-206, ADR-0028 §17): whether this browser can receive
// pushes (a Service Worker and the Push API), what the reader said to the site's notifications, asking them, and the
// browser's push subscription, made with the server's application key through the app's Service Worker. The app's
// badge shows a count where the platform has one (the Badging API), and nothing elsewhere. A browser that has none of
// them answers so, and does nothing.

/** What the reader said to this site's notifications, or that the browser has none. */
export type NotificationPermissionState = "default" | "granted" | "denied" | "unsupported";

/** A browser's push subscription as the server takes it (`PushSubscription.toJSON()`). */
export type PushSubscriptionData = { endpoint: string; keys: { p256dh: string; auth: string } };

/** Whether this browser can receive Web Push: it has a Service Worker and the Push API, and notifications. */
export function supportsPush(): boolean {
  return (
    typeof navigator !== "undefined" &&
    "serviceWorker" in navigator &&
    typeof PushManager !== "undefined" &&
    typeof Notification !== "undefined"
  );
}

/** What the reader said to this site's notifications. */
export function notificationPermission(): NotificationPermissionState {
  if (typeof Notification === "undefined") return "unsupported";
  return Notification.permission;
}

/**
 * Asks the reader whether this site may show notifications, as a press of theirs asks (never on its own); what they
 * answered, `default` when they closed the question without choosing.
 */
export async function askNotificationPermission(): Promise<NotificationPermissionState> {
  if (typeof Notification === "undefined") return "unsupported";
  try {
    return await Notification.requestPermission();
  } catch {
    return Notification.permission;
  }
}

/**
 * Registers the app's Service Worker at `url` (the build's /sw.js), which shows the pushes and opens what a click on
 * one tells of; nothing where the browser has no Service Worker. A failure leaves the app as it is: it then gets no
 * pushes.
 */
export function registerServiceWorker(url: string): void {
  if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return;
  void navigator.serviceWorker.register(url, { scope: "/" }).catch(() => undefined);
}

const fromBase64Url = (text: string): Uint8Array<ArrayBuffer> =>
  Uint8Array.from(atob(text.replaceAll("-", "+").replaceAll("_", "/")), (c) => c.charCodeAt(0));

const toBase64Url = (buffer: ArrayBuffer | null): string =>
  buffer === null
    ? ""
    : btoa(String.fromCharCode(...new Uint8Array(buffer)))
        .replaceAll("+", "-")
        .replaceAll("/", "_")
        .replace(/=+$/, "");

/** Whether `subscription` was made with the application key `key` (base64url). */
function madeWith(subscription: PushSubscription, key: string): boolean {
  return toBase64Url(subscription.options.applicationServerKey) === key;
}

/**
 * Subscribes this browser to the pushes of the server whose application key is `key` (base64url), through the app's
 * Service Worker, and returns the subscription to register with the server. A subscription the browser holds with
 * another key (the server's changed) is ended first, since a browser keeps one. It rejects when the browser refuses.
 */
export async function subscribePush(key: string): Promise<PushSubscriptionData> {
  const registration = await navigator.serviceWorker.ready;
  const held = await registration.pushManager.getSubscription();
  if (held !== null && !madeWith(held, key)) await held.unsubscribe();
  const subscription =
    held !== null && madeWith(held, key)
      ? held
      : await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: fromBase64Url(key) });
  return {
    endpoint: subscription.endpoint,
    keys: {
      p256dh: toBase64Url(subscription.getKey("p256dh")),
      auth: toBase64Url(subscription.getKey("auth")),
    },
  };
}

/** Shows `count` on the app's badge, or clears it at 0, where the platform has a badge; nothing elsewhere. */
export function showAppBadge(count: number): void {
  // The DOM's types declare the Badging API, which only some browsers have (Chromium's installed apps, Safari's).
  if (typeof navigator === "undefined" || !("setAppBadge" in navigator)) return;
  if (count > 0) void navigator.setAppBadge(count).catch(() => undefined);
  else void navigator.clearAppBadge().catch(() => undefined);
}
