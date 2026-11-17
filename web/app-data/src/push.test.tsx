// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import {
  notificationSettingsQuery,
  pushSubscriptionQuery,
  rememberPushPromptClosed,
  useNotificationKinds,
  usePushSubscription,
  useRegisterPush,
  type PushSubscription,
} from "./push";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

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
