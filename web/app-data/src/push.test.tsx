// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { createFakeApi, fakeOrigin } from "@my/e2e/fake";
import type { AppData } from "./app-data";
import {
  notificationSettingsQuery,
  notificationSoundOn,
  pushSubscriptionQuery,
  rememberPushPromptClosed,
  setNotificationSound,
  setNotificationSoundKinds,
  turnPushOff,
  turnPushOn,
  useNotificationKinds,
  usePushSubscription,
  useRegisterPush,
  watchNotificationSound,
  type PushSubscription,
} from "./push";
import { startStream } from "./stream";
import { dataOn, fakeData } from "./test/data";
import { probe } from "./test/probe";
import { refusedWith } from "./test/refused";
import { fakeStreams } from "./test/tabs";

describe("usePushSubscription", () => {
  test("is undefined until it is read, the server's once read, and null when the server has none (a 404)", async () => {
    const found = fakeData({ push: true, pushState: "subscribed" }).data;
    expect(probe(found, usePushSubscription)).toBeUndefined();
    const subscription = await found.queryClient.query(pushSubscriptionQuery(found));
    expect(subscription).toMatchObject({ endpoint: "https://push.example/s/held", alerts: ["mention"] });
    expect(probe(found, usePushSubscription)).toEqual(subscription);

    const none = fakeData({ push: true }).data;
    await none.queryClient.query(pushSubscriptionQuery(none));
    expect(probe(none, usePushSubscription)).toBeNull();
  });

  test("is undefined when it could not be read, never null", async () => {
    const { data } = fakeData({ push: true, pushState: "subscription-fails" });
    await data.queryClient.query(pushSubscriptionQuery(data)).catch(() => undefined);
    expect(probe(data, usePushSubscription)).toBeUndefined();
  });
});

describe("useNotificationKinds", () => {
  test("is every kind of the reader's rows, each once, and undefined until the rows are read", async () => {
    const { fake, data } = fakeData({ push: true });
    // The server lists the quotes with the boosts, in one row of two kinds, and in a row of their own.
    fake.script("GET /preferences/notifications", {
      change: (settings) => ({
        rows: settings.rows.map((row) =>
          row.group === "boost" ? { ...row, types: ["boost" as const, "quote" as const] } : row,
        ),
      }),
    });
    expect(probe(data, useNotificationKinds)).toBeUndefined();
    await data.queryClient.query(notificationSettingsQuery(data));
    const kinds = probe(data, useNotificationKinds);
    expect([...(kinds ?? [])].sort()).toEqual([
      "boost",
      "favourite",
      "follow",
      "follow_request",
      "mention",
      "quote",
      "reaction",
    ]);
  });

  test("stays undefined when the rows could not be read", async () => {
    const { data } = fakeData({ push: true, pushState: "settings-fail" });
    await data.queryClient.query(notificationSettingsQuery(data)).catch(() => undefined);
    expect(probe(data, useNotificationKinds)).toBeUndefined();
  });
});

describe("useRegisterPush", () => {
  const browser = { endpoint: "https://push.example/s/1", keys: { p256dh: "-_8B", auth: "_gI" } };

  test("puts this browser's subscription for the kinds given, and the server's answer is the subscription then", async () => {
    const { fake, data } = fakeData({ push: true });
    const sent = fake.script("PUT /push/subscription");
    await data.queryClient.query(pushSubscriptionQuery(data));
    expect(probe(data, usePushSubscription)).toBeNull();
    const register = probe(data, useRegisterPush);
    await register(browser, ["mention", "follow"]);
    expect(sent.seen.map((s) => s.body)).toEqual([{ ...browser, alerts: ["mention", "follow"] }]);
    const { data: kept } = await data.api.GET("/push/subscription");
    expect(kept).toMatchObject({ endpoint: browser.endpoint, alerts: ["mention", "follow"] });
    expect(probe(data, usePushSubscription)).toEqual(kept as PushSubscription);
  });

  test("rejects when the server refuses, and the grant still has none", async () => {
    const { fake, data } = fakeData({ push: true });
    fake.script("PUT /push/subscription", { fail: 422 });
    await data.queryClient.query(pushSubscriptionQuery(data));
    let refused: unknown;
    await probe(data, useRegisterPush)(browser, ["mention"]).catch((e: unknown) => {
      refused = e;
    });
    expect(refused).toBeDefined();
    expect(probe(data, usePushSubscription)).toBeNull();
  });
});

/** A device's storage that keeps what it is given; Bun has none of its own. */
function deviceStorage(kept = new Map<string, string>()): Storage {
  return {
    get length() {
      return kept.size;
    },
    clear: () => {
      kept.clear();
    },
    getItem: (k) => kept.get(k) ?? null,
    key: (i) => Array.from(kept.keys())[i] ?? null,
    removeItem: (k) => {
      kept.delete(k);
    },
    setItem: (k, v) => {
      kept.set(k, v);
    },
  };
}

// What usePushPromptClosed answers, account by account, the prompt's tests check on a DOM
// (web/app-features/src/notifications/push-prompt.dom.test.tsx): a render without a DOM reads no device.
describe("rememberPushPromptClosed", () => {
  const key = "my:push-prompt-closed";
  let kept: Map<string, string>;
  /** The accounts the device keeps as having closed the prompt (ADR-0086 §21: a list of account IDs). */
  const closedOnDevice = (): unknown => {
    const value = kept.get(key);
    return value === undefined ? [] : (JSON.parse(value) as unknown);
  };

  beforeEach(() => {
    kept = new Map();
    globalThis.localStorage = deviceStorage(kept);
  });
  afterEach(() => {
    Reflect.deleteProperty(globalThis, "localStorage");
  });

  test("keeps on the device each account that closed the prompt, once", () => {
    rememberPushPromptClosed("yuki", true);
    rememberPushPromptClosed("hana", true);
    rememberPushPromptClosed("yuki", true);
    expect(closedOnDevice()).toEqual(expect.arrayContaining(["yuki", "hana"]));
    expect(closedOnDevice()).toHaveLength(2);
  });

  test("taking one account's record away leaves the others'", () => {
    rememberPushPromptClosed("yuki", true);
    rememberPushPromptClosed("hana", true);
    rememberPushPromptClosed("yuki", false);
    rememberPushPromptClosed("marco", false);
    expect(closedOnDevice()).toEqual(["hana"]);
  });

  test("a value the device cannot read is replaced, and a device that keeps nothing does not fail", () => {
    kept.set(key, "{not json");
    rememberPushPromptClosed("yuki", true);
    expect(closedOnDevice()).toEqual(["yuki"]);
    const refuse = () => {
      throw new DOMException("The storage is blocked", "SecurityError");
    };
    globalThis.localStorage = {
      length: 0,
      clear: refuse,
      getItem: refuse,
      key: refuse,
      removeItem: refuse,
      setItem: refuse,
    };
    expect(() => {
      rememberPushPromptClosed("yuki", true);
    }).not.toThrow();
  });
});

describe("turnPushOff", () => {
  let kept: Map<string, string>;
  beforeEach(() => {
    kept = new Map();
    globalThis.localStorage = deviceStorage(kept);
  });
  afterEach(() => {
    Reflect.deleteProperty(globalThis, "localStorage");
  });

  test("deletes the server's subscription, and the cache holds none; the prompt stays closed on this device", async () => {
    const { data, requests } = fakeData({ push: true, pushState: "subscribed" });
    await data.queryClient.query(pushSubscriptionQuery(data));
    expect(probe(data, usePushSubscription)).not.toBeNull();
    rememberPushPromptClosed("yuki", true);
    await turnPushOff(data, "yuki");
    expect(requests.filter((r) => !r.startsWith("GET "))).toEqual(["DELETE /push/subscription"]);
    expect(probe(data, usePushSubscription)).toBeNull();
    expect(JSON.parse(kept.get("my:push-prompt-closed") ?? "[]")).toEqual(["yuki"]);
  });

  test("rejects when the server refuses, and the cache keeps the subscription", async () => {
    const { fake, data } = fakeData({ push: true, pushState: "subscribed" });
    fake.script("DELETE /push/subscription", { fail: 500 });
    await data.queryClient.query(pushSubscriptionQuery(data));
    const before = probe(data, usePushSubscription);
    await refusedWith(turnPushOff(data, "yuki"));
    expect(probe(data, usePushSubscription)).toEqual(before);
  });
});

describe("the notification sound", () => {
  beforeEach(() => {
    globalThis.localStorage = deviceStorage();
  });
  afterEach(() => {
    Reflect.deleteProperty(globalThis, "localStorage");
  });

  test("is off until the device is told otherwise, and then is what it was told", () => {
    expect(notificationSoundOn()).toBe(false);
    setNotificationSound(true);
    expect(notificationSoundOn()).toBe(true);
    setNotificationSound(false);
    expect(notificationSoundOn()).toBe(false);
  });

  test("is off on a device that keeps nothing, and setting it does not fail", () => {
    const refuse = () => {
      throw new DOMException("The storage is blocked", "SecurityError");
    };
    globalThis.localStorage = {
      length: 0,
      clear: refuse,
      getItem: refuse,
      key: refuse,
      removeItem: refuse,
      setItem: refuse,
    };
    expect(notificationSoundOn()).toBe(false);
    expect(() => {
      setNotificationSound(true);
    }).not.toThrow();
  });
});

// Which kinds sound is the device's too (ADR-0086 §12): a notification that arrives on the stream plays the sound when
// the sound is on and its kind's is. What the settings' Sound column shows of the kinds is the feature's test, on a DOM
// (web/app-features/src/settings/notifications-settings.dom.test.tsx): a render without one reads no device.
describe("watchNotificationSound", () => {
  let stops: (() => void)[];
  beforeEach(() => {
    globalThis.localStorage = deviceStorage();
    stops = [];
  });
  afterEach(() => {
    for (const stop of stops) stop();
    Reflect.deleteProperty(globalThis, "localStorage");
  });

  /**
   * A reader whose stream is open, and who is told of each sound: `arrive()` is a new notification on the stream (a
   * favourite of their post), `played()` how many sounds they heard, and `stop()` ends the telling.
   */
  async function listening() {
    const fake = createFakeApi();
    const data: AppData = { ...dataOn(fake).data, connection: { server: fakeOrigin } };
    const { run, streams } = fakeStreams();
    stops.push(startStream(data, { tabs: null, run }));
    await Bun.sleep(0);
    const [stream] = streams;
    if (stream === undefined) throw new Error("test: no stream was opened");
    stream.open(false);
    let played = 0;
    const stop = watchNotificationSound(data, () => (played += 1));
    stops.push(stop);
    return {
      arrive: () => {
        stream.send({ type: "notification", notification: fake.newNotification() });
      },
      played: () => played,
      stop,
    };
  }

  test("plays nothing while the sound is off, whichever kinds have theirs on", async () => {
    setNotificationSoundKinds(["favourite"], true);
    const { arrive, played } = await listening();

    arrive();

    expect(played()).toBe(0);
  });

  test("plays nothing for a kind whose sound is off, and no kind's is on until the device is told", async () => {
    setNotificationSound(true);
    const { arrive, played } = await listening();

    arrive();
    expect(played()).toBe(0);

    setNotificationSoundKinds(["mention", "follow"], true);
    arrive();
    expect(played()).toBe(0);
  });

  test("plays once for a notification of a kind whose sound is on, and once for several that arrive together", async () => {
    setNotificationSound(true);
    setNotificationSoundKinds(["mention", "favourite"], true);
    const { arrive, played } = await listening();

    arrive();
    expect(played()).toBe(1);

    arrive();
    arrive();
    expect(played()).toBe(1);
  });

  test("a kind whose sound was turned off again plays nothing, and the other kinds keep theirs", async () => {
    setNotificationSound(true);
    setNotificationSoundKinds(["mention", "favourite"], true);
    setNotificationSoundKinds(["favourite"], false);
    const silent = await listening();

    silent.arrive();
    expect(silent.played()).toBe(0);

    setNotificationSoundKinds(["favourite"], true);
    silent.arrive();
    expect(silent.played()).toBe(1);
  });

  test("the function it returns ends it: a notification that arrives after plays nothing", async () => {
    setNotificationSound(true);
    setNotificationSoundKinds(["favourite"], true);
    const { arrive, played, stop } = await listening();

    stop();
    arrive();

    expect(played()).toBe(0);
  });
});

describe("turnPushOn", () => {
  afterEach(() => {
    Reflect.deleteProperty(globalThis, "Notification");
  });

  for (const [permission, outcome] of [
    ["denied", "blocked"],
    ["default", "closed"],
  ] as const)
    test(`is ${outcome} when the person's answer to the browser's question is ${permission}, and asks the server nothing`, async () => {
      const { data, requests } = fakeData({ push: true });
      Object.assign(globalThis, {
        Notification: { permission: "default", requestPermission: () => Promise.resolve(permission) },
      });
      expect(await turnPushOn(data, "yuki", "key", ["mention"])).toBe(outcome);
      expect(requests.filter((r) => !r.startsWith("GET "))).toEqual([]);
    });
});
