// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import {
  askNotificationPermission,
  notificationPermission,
  showAppBadge,
  subscribePush,
  supportsPush,
  watchNotificationPermission,
} from "./push";

/** The server's application key as the instance names it (base64url), and its bytes. */
const key = "BAECAwQFBgcICQ";
const keyBytes = [4, 1, 2, 3, 4, 5, 6, 7, 8, 9];
const otherKey = "BAkICAcGBQQDAgE";

const bytes = (values: readonly number[]) => new Uint8Array(values).buffer;
const listOf = (source: BufferSource | string | null | undefined): number[] => {
  if (source === null || source === undefined || typeof source === "string") return [];
  const view =
    source instanceof ArrayBuffer
      ? new Uint8Array(source)
      : new Uint8Array(source.buffer, source.byteOffset, source.byteLength);
  return Array.from(view);
};

/** A subscription the browser holds, made with the key of `made` (base64url). */
function heldSubscription(made: string, endpoint: string) {
  const decoded = Uint8Array.from(atob(made.replaceAll("-", "+").replaceAll("_", "/")), (c) => c.charCodeAt(0));
  const subscription = {
    endpoint,
    expirationTime: null,
    options: { userVisibleOnly: true, applicationServerKey: decoded.buffer },
    // 0xfb 0xff: bytes whose base64 has + and /, which base64url writes as - and _.
    getKey: (name: string) => (name === "p256dh" ? bytes([0xfb, 0xff, 1]) : name === "auth" ? bytes([0xfe, 2]) : null),
    toJSON: () => ({ endpoint, expirationTime: null, keys: { p256dh: "-_8B", auth: "_gI" } }),
    unsubscribe: mock(() => Promise.resolve(true)),
  };
  return subscription;
}

type Held = ReturnType<typeof heldSubscription>;

/**
 * A browser with a Service Worker, the Push API and notifications, whose push manager holds `held` and subscribes with
 * `subscribe`; `restore` takes them away.
 */
function installBrowser(held: Held | null, subscribe: () => Promise<Held | null> = () => Promise.resolve(held)) {
  let current = held;
  const order: string[] = [];
  const pushManager = {
    getSubscription: mock(() => Promise.resolve(current)),
    subscribe: mock(async (options: PushSubscriptionOptionsInit) => {
      order.push("subscribe");
      expect(options.userVisibleOnly).toBe(true);
      const made = await subscribe();
      current = made;
      return made;
    }),
  };
  if (held !== null)
    held.unsubscribe.mockImplementation(() => {
      order.push("unsubscribe");
      current = null;
      return Promise.resolve(true);
    });
  class FakeRegistration {
    readonly pushManager = pushManager;
    readonly active = {};
    showNotification() {
      return Promise.resolve();
    }
  }
  const registration = new FakeRegistration();
  const serviceWorker = {
    ready: Promise.resolve(registration),
    controller: {},
    getRegistration: () => Promise.resolve(registration),
    register: () => Promise.resolve(registration),
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  };
  Object.defineProperty(navigator, "serviceWorker", { configurable: true, value: serviceWorker });
  Object.assign(globalThis, {
    window: globalThis,
    PushManager: function PushManager() {
      return undefined;
    },
    ServiceWorkerRegistration: FakeRegistration,
    Notification: { permission: "granted", requestPermission: () => Promise.resolve("granted") },
  });
  return { pushManager, order };
}

afterEach(() => {
  for (const name of ["window", "PushManager", "ServiceWorkerRegistration", "Notification"])
    Reflect.deleteProperty(globalThis, name);
  Reflect.deleteProperty(navigator, "serviceWorker");
  Reflect.deleteProperty(navigator, "setAppBadge");
  Reflect.deleteProperty(navigator, "clearAppBadge");
});

describe("supportsPush", () => {
  test("a browser with a Service Worker, the Push API and notifications can receive pushes", () => {
    installBrowser(null);
    expect(supportsPush()).toBe(true);
  });

  test("a browser without one of them cannot", () => {
    for (const missing of ["PushManager", "Notification", "serviceWorker"]) {
      installBrowser(null);
      if (missing === "serviceWorker") Reflect.deleteProperty(navigator, "serviceWorker");
      else Reflect.deleteProperty(globalThis, missing);
      expect([missing, supportsPush()]).toEqual([missing, false]);
    }
    Reflect.deleteProperty(globalThis, "window");
    Reflect.deleteProperty(navigator, "serviceWorker");
    expect(supportsPush()).toBe(false);
  });
});

describe("notificationPermission and askNotificationPermission", () => {
  test("say what the reader said, and that a browser without notifications has none", async () => {
    expect(notificationPermission()).toBe("unsupported");
    expect(await askNotificationPermission()).toBe("unsupported");
    for (const said of ["default", "granted", "denied"] as const) {
      Object.assign(globalThis, { Notification: { permission: said } });
      expect(notificationPermission()).toBe(said);
    }
  });

  test("asking answers what the question answered, default when it closed without a choice", async () => {
    for (const answer of ["default", "granted", "denied"] as const) {
      const requestPermission = mock(() => Promise.resolve(answer));
      Object.assign(globalThis, { Notification: { permission: "default", requestPermission } });
      expect(await askNotificationPermission()).toBe(answer);
      expect(requestPermission).toHaveBeenCalledTimes(1);
    }
  });
});

describe("watchNotificationPermission", () => {
  /** What the browser says of the site's notifications, which its settings change without a question. */
  class SitePermission extends EventTarget {
    state: PermissionState = "prompt";
    onchange: ((event: Event) => void) | null = null;
    /** The reader changed the site's permission in the browser's settings. */
    set(state: PermissionState) {
      this.state = state;
      const event = new Event("change");
      this.dispatchEvent(event);
      this.onchange?.(event);
    }
  }

  /** A browser with notifications whose question answers `answer`; with the Permissions API unless `permissions` is false. */
  function browser(answer: NotificationPermission, permissions = true) {
    const site = new SitePermission();
    const asked: unknown[] = [];
    Object.assign(globalThis, {
      Notification: { permission: "default", requestPermission: () => Promise.resolve(answer) },
    });
    if (permissions)
      Object.defineProperty(navigator, "permissions", {
        configurable: true,
        value: {
          query: (descriptor: unknown) => {
            asked.push(descriptor);
            return Promise.resolve(site);
          },
        },
      });
    return { site, asked };
  }

  /** A watcher, how many times it was told, and the function that stops it, once the browser answered the watch. */
  async function watching() {
    const told = { times: 0 };
    const stop = watchNotificationPermission(() => {
      told.times += 1;
    });
    await Bun.sleep(0);
    return { told, stop };
  }

  afterEach(() => {
    Reflect.deleteProperty(navigator, "permissions");
  });

  for (const answer of ["granted", "denied", "default"] as const)
    test(`tells its watcher after each question the app asks, here answered ${answer}`, async () => {
      browser(answer);
      const { told, stop } = await watching();
      expect(told.times).toBe(0);

      expect(await askNotificationPermission()).toBe(answer);
      expect(told.times).toBe(1);

      await askNotificationPermission();
      expect(told.times).toBe(2);
      stop();
    });

  test("tells its watcher each time the browser's settings change what the site may do, with no question asked", async () => {
    const { site, asked } = browser("granted");
    const { told, stop } = await watching();
    expect(asked).toEqual([{ name: "notifications" }]);
    expect(told.times).toBe(0);

    site.set("denied");
    expect(told.times).toBe(1);

    site.set("prompt");
    expect(told.times).toBe(2);
    stop();
  });

  test("a browser without the Permissions API is followed by the app's questions alone", async () => {
    browser("denied", false);
    const { told, stop } = await watching();

    await askNotificationPermission();

    expect(told.times).toBe(1);
    stop();
  });

  test("the function it returns stops it: neither a question nor a change of the browser's settings tells the watcher", async () => {
    const { site } = browser("granted");
    const { told, stop } = await watching();

    stop();
    await askNotificationPermission();
    site.set("denied");

    expect(told.times).toBe(0);
  });

  test("each watcher is told, and stopping one leaves the other", async () => {
    const { site } = browser("granted");
    const first = await watching();
    const second = await watching();

    await askNotificationPermission();
    expect([first.told.times, second.told.times]).toEqual([1, 1]);

    first.stop();
    site.set("denied");
    await askNotificationPermission();

    expect([first.told.times, second.told.times]).toEqual([1, 3]);
    second.stop();
  });
});

describe("subscribePush", () => {
  test("subscribes with the server's key, visible only, and returns the subscription as the server takes it", async () => {
    const made = heldSubscription(key, "https://push.example/s/1");
    const { pushManager } = installBrowser(null, () => Promise.resolve(made));
    expect(await subscribePush(key)).toEqual({
      endpoint: "https://push.example/s/1",
      keys: { p256dh: "-_8B", auth: "_gI" },
    });
    expect(pushManager.subscribe).toHaveBeenCalledTimes(1);
    const options = pushManager.subscribe.mock.calls[0]?.[0];
    expect(options?.userVisibleOnly).toBe(true);
    expect(listOf(options?.applicationServerKey)).toEqual(keyBytes);
  });

  test("ends a subscription the browser holds with another key before it subscribes with the server's", async () => {
    const old = heldSubscription(otherKey, "https://push.example/s/old");
    const made = heldSubscription(key, "https://push.example/s/new");
    const { pushManager, order } = installBrowser(old, () => Promise.resolve(made));
    const subscription = await subscribePush(key);
    expect(old.unsubscribe).toHaveBeenCalledTimes(1);
    expect(order).toEqual(["unsubscribe", "subscribe"]);
    expect(listOf(pushManager.subscribe.mock.calls[0]?.[0].applicationServerKey)).toEqual(keyBytes);
    expect(subscription.endpoint).toBe("https://push.example/s/new");
  });

  test("keeps a subscription the browser holds with the server's key", async () => {
    const held = heldSubscription(key, "https://push.example/s/1");
    installBrowser(held);
    const subscription = await subscribePush(key);
    expect(held.unsubscribe).not.toHaveBeenCalled();
    expect(subscription.endpoint).toBe("https://push.example/s/1");
  });

  test("rejects when the browser refuses", async () => {
    installBrowser(null, () => Promise.reject(new DOMException("denied", "NotAllowedError")));
    let refused: unknown;
    await subscribePush(key).catch((e: unknown) => {
      refused = e;
    });
    expect(refused).toBeDefined();
  });

  test("rejects at once when the page has no Service Worker registration, and asks the push manager nothing", async () => {
    const { pushManager } = installBrowser(null, () =>
      Promise.resolve(heldSubscription(key, "https://push.example/s/1")),
    );
    // A browser's `ready` waits until a worker is registered, which never happens here.
    Object.defineProperty(navigator, "serviceWorker", {
      configurable: true,
      value: {
        ready: new Promise(() => undefined),
        controller: null,
        getRegistration: () => Promise.resolve(undefined),
        register: () => new Promise(() => undefined),
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      },
    });
    const outcome = await Promise.race([
      subscribePush(key).then(
        () => "subscribed",
        () => "rejected",
      ),
      Bun.sleep(50).then(() => "still waiting"),
    ]);
    expect(outcome).toBe("rejected");
    expect(pushManager.subscribe).not.toHaveBeenCalled();
  });
});

describe("showAppBadge", () => {
  const installBadge = () => {
    const setAppBadge = mock((count?: number) => Promise.resolve(count).then(() => undefined));
    const clearAppBadge = mock(() => Promise.resolve());
    Object.defineProperty(navigator, "setAppBadge", { configurable: true, value: setAppBadge });
    Object.defineProperty(navigator, "clearAppBadge", { configurable: true, value: clearAppBadge });
    return { setAppBadge, clearAppBadge };
  };

  test("shows the count on the app's badge", () => {
    const { setAppBadge, clearAppBadge } = installBadge();
    showAppBadge(3);
    expect(setAppBadge.mock.calls).toEqual([[3]]);
    expect(clearAppBadge).not.toHaveBeenCalled();
  });

  test("clears the badge at 0, and shows no count of 0", () => {
    const { setAppBadge, clearAppBadge } = installBadge();
    showAppBadge(0);
    const cleared = clearAppBadge.mock.calls.length + setAppBadge.mock.calls.filter(([n]) => n === 0).length;
    expect(cleared).toBe(1);
    expect(setAppBadge.mock.calls.filter(([n]) => n !== 0)).toEqual([]);
  });

  test("does nothing where the platform has no badge", () => {
    expect(() => {
      showAppBadge(3);
      showAppBadge(0);
    }).not.toThrow();
  });
});
