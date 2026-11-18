// SPDX-License-Identifier: AGPL-3.0-only

// The browser as a story about Web Push and the notification sound needs it (ADR-0086 §12, §18–§22), stubbed before
// the story renders, as a headless browser blocks notifications: the site's notifications at a permission, a push
// manager behind a Service Worker, or a browser with no Web Push at all, as a tab of Safari on an iPhone is; and what
// the device keeps, which a story starts from and ends without: who closed the prompt, whether the notification
// sound is on, and the kinds that sound. Only @my/platform reads the platform's globals in the app, so the stubs
// reach them as properties of the window.

/** The device's keys of the accounts that closed the prompt (ADR-0086 §21), of the sound, and of the kinds that sound. */
const closedKey = "my:push-prompt-closed";
const soundKey = "my:notification-sound";
const soundKindsKey = "my:notification-sound-kinds";

/** The browser a story is shown in. */
export type PushBrowser = {
  /** What the reader said to the site's notifications: never asked by default. */
  permission?: NotificationPermission;
  /**
   * What the browser has of Web Push: all of it by default; none (`none`); or none as a tab of Safari on an iPhone,
   * which has it once the site is on the Home Screen (`install`).
   */
  push?: "none" | "install";
  /** The kinds of notifications that sound on the device; without it, the notification sound is off. */
  sounding?: readonly string[];
};

/** A subscription as the browser holds one, made with the application key `key`. */
function subscriptionOf(key: BufferSource | string | null | undefined) {
  const endpoint = "https://push.example/s/story";
  const applicationServerKey =
    key === null || key === undefined || typeof key === "string"
      ? null
      : key instanceof ArrayBuffer
        ? key
        : new Uint8Array(key.buffer, key.byteOffset, key.byteLength).slice().buffer;
  return {
    endpoint,
    expirationTime: null,
    options: { userVisibleOnly: true, applicationServerKey },
    getKey: (name: string) =>
      name === "p256dh" ? new Uint8Array([4, 1, 2]).buffer : name === "auth" ? new Uint8Array([9, 9]).buffer : null,
    toJSON: () => ({ endpoint, expirationTime: null, keys: { p256dh: "BAEC", auth: "CQk" } }),
    unsubscribe: () => Promise.resolve(true),
  };
}

/** Puts `value` in place of the property `name` of `target`, and returns what puts the property back. */
function replace(target: object, name: string, descriptor: PropertyDescriptor | undefined): () => void {
  const was = Object.getOwnPropertyDescriptor(target, name);
  if (descriptor === undefined) Reflect.deleteProperty(target, name);
  else Object.defineProperty(target, name, { configurable: true, ...descriptor });
  return () => {
    if (was === undefined) Reflect.deleteProperty(target, name);
    else Object.defineProperty(target, name, was);
  };
}

/**
 * The `beforeEach` of a story in the browser `browser`: stubs it before the story renders, and returns what puts the
 * browser back.
 */
export const pushBrowser =
  ({ permission = "default", push, sounding }: PushBrowser = {}) =>
  () => {
    const forget = () => {
      for (const key of [closedKey, soundKey, soundKindsKey]) localStorage.removeItem(key);
    };
    forget();
    if (sounding !== undefined) {
      localStorage.setItem(soundKey, "on");
      if (sounding.length > 0) localStorage.setItem(soundKindsKey, JSON.stringify(sounding));
    }
    const undo: (() => void)[] = [forget];
    const device = Reflect.get(window, "navigator") as object;
    if (push !== undefined) {
      // No Push API: the manager's constructor is what the platform asks for.
      undo.push(replace(window, "PushManager", undefined));
      if (push === "install") {
        undo.push(replace(device, "platform", { get: () => "iPhone" }));
        undo.push(replace(device, "standalone", { get: () => false }));
      }
    } else {
      const notifications = Reflect.get(window, "Notification") as object;
      let now = permission;
      undo.push(replace(notifications, "permission", { get: () => now }));
      undo.push(
        replace(notifications, "requestPermission", {
          value: () => {
            now = "granted";
            return Promise.resolve(now);
          },
        }),
      );
      let current: ReturnType<typeof subscriptionOf> | null = null;
      const pushManager = {
        getSubscription: () => Promise.resolve(current),
        permissionState: () => Promise.resolve(now === "granted" ? "granted" : now === "denied" ? "denied" : "prompt"),
        subscribe: (options?: PushSubscriptionOptionsInit) => {
          current = subscriptionOf(options?.applicationServerKey);
          return Promise.resolve(current);
        },
      };
      const registration = {
        scope: `${location.origin}/`,
        active: {},
        pushManager,
        showNotification: () => Promise.resolve(),
        getNotifications: () => Promise.resolve([]),
      };
      const container = Reflect.get(device, "serviceWorker") as object;
      undo.push(replace(container, "ready", { get: () => Promise.resolve(registration) }));
      undo.push(replace(container, "getRegistration", { value: () => Promise.resolve(registration) }));
      undo.push(replace(container, "getRegistrations", { value: () => Promise.resolve([registration]) }));
    }
    return () => {
      for (const put of undo.toReversed()) put();
    };
  };
