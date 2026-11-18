// SPDX-License-Identifier: AGPL-3.0-only

// Notifications, the entry of the settings (#3201): what is this device's. Web Push (ADR-0086 §8, §18–§21): a switch
// that asks the browser and registers a subscription for every kind of the reader's rows, as the prompt over the
// notifications does, and that deletes it; where it cannot be turned on it stands off and disabled and says why; while
// the device has a subscription, the tables' Push column changes the kinds it is for. The notification sound (§12):
// off until the reader turns it on, kept on this device with the kinds it plays for, never sent to the server, and
// tried with a button. The browser is stubbed whole (test/browser.ts): what its own parts answer is the platform's
// test, and that a subscription reaches a real server, the E2E test's.

import { describe, expect, test } from "bun:test";
import { rememberPushPromptClosed, usePushPromptClosed } from "@my/app-data";
import { memberNotificationRows, type FakeOptions } from "@my/e2e/fake";
import { renderHook, screen, waitFor } from "@testing-library/react";
import type { PushBrowser } from "../test/browser";
import { fakeData } from "../test/feature";
import {
  cell,
  columnsOf,
  describedBy,
  drawnOn,
  kindsOf,
  people,
  readSettings,
  said,
  section,
  server,
  stateOf,
  toggle,
  type Device,
  type Drawn,
} from "../test/notification-settings";

const push = "Push notifications";
const sound = "Notification sound";
const device = "This device";

/** A member's rows on a server that sends Web Push, to a reader with no subscription. */
const sends = { push: true, settings: { notificationRows: memberNotificationRows } } as const satisfies FakeOptions;
/** The same, to a reader whose grant holds a subscription for mentions. */
const subscribed = { ...sends, pushState: "subscribed" } as const satisfies FakeOptions;

/** Every kind of notification a member's rows hold. */
const everyKind = kindsOf(memberNotificationRows);

/** The entry once the reader's subscription is read too: the switch is at what the server holds. */
async function withPush(
  options: FakeOptions,
  browser: PushBrowser = {},
  sounding?: Device["sounding"],
): Promise<Drawn> {
  const drawn = await readSettings(options, sounding === undefined ? { browser } : { browser, sounding });
  await waitFor(() => {
    if (drawn.data.queryClient.isFetching() > 0) throw new Error("test: the screen is still reading");
  });
  return drawn;
}

/** The writes the app sent, each as `METHOD /path`. */
const writes = ({ requests }: Pick<Drawn, "requests">) => requests.filter((request) => !request.startsWith("GET "));

/** Whether this device keeps that the account of `id` closed the Web Push prompt. */
const promptClosed = (id: string) => renderHook(() => usePushPromptClosed(id)).result.current;

/** What the rows of This device say: of a save, or of why something did not happen. */
const saidOnDevice = () => said(section(device).queryAllByRole("status"));

const blocked = "Your browser blocks notifications from this site. Allow them in the browser's settings.";
const notTurnedOn = "Notifications could not be turned on. Reload or update the app, then try again.";

describe("Web Push on this device", () => {
  test("on a device that can have it and has none, the switch is off, the reader's to turn on, and says what it does", async () => {
    const { seen } = await withPush(sends);

    expect(toggle(push).checked).toBe(false);
    expect(toggle(push).disabled).toBe(false);
    expect(describedBy(toggle(push))).toEqual(["Get your notifications on this device, even while the app is closed."]);
    // Drawing the screen asks the browser nothing.
    expect(seen).toEqual({ asked: 0, subscribed: 0, played: 0 });
    expect(columnsOf(people)).not.toContain("Push");
  });

  test("turned on, it asks the browser, registers a subscription for every kind of the reader's rows, and the tables gain Push", async () => {
    const drawn = await withPush(sends);
    const { fake, user, seen } = drawn;
    const sent = fake.script("PUT /push/subscription");

    await user.click(toggle(push));

    const { body } = await sent.served();
    expect(body.endpoint).toBe("https://push.example/s/test");
    expect([...body.alerts].sort()).toEqual(everyKind);
    await waitFor(() => {
      expect(columnsOf(people)).toContain("Push");
    });
    expect(toggle(push).checked).toBe(true);
    expect(seen).toMatchObject({ asked: 1, subscribed: 1 });
    expect(writes(drawn)).toEqual(["PUT /push/subscription"]);
    // Every kind is on: the first row of each table says so.
    expect(stateOf(cell(people, "All", "Push"))).toBe("on");
    expect(stateOf(cell(server, "All", "Push"))).toBe("on");
  });

  test("where the site's notifications were allowed before, it subscribes without asking again", async () => {
    const { fake, user, seen } = await withPush(sends, { permission: "granted" });
    const sent = fake.script("PUT /push/subscription");

    await user.click(toggle(push));
    await sent.served();

    expect(seen).toMatchObject({ asked: 0, subscribed: 1 });
  });

  test("turned off, it deletes the subscription, and the tables lose Push", async () => {
    const drawn = await withPush(subscribed, { permission: "granted" });
    const { fake, user } = drawn;
    expect(toggle(push).checked).toBe(true);
    expect(columnsOf(people)).toContain("Push");
    const sent = fake.script("DELETE /push/subscription");

    await user.click(toggle(push));

    await sent.served();
    await waitFor(() => {
      expect(columnsOf(people)).not.toContain("Push");
    });
    expect(toggle(push).checked).toBe(false);
    expect(toggle(push).disabled).toBe(false);
    expect(writes(drawn)).toEqual(["DELETE /push/subscription"]);
  });

  // ADR-0086 §21: the prompt over the notifications and this switch keep one record of who closed the prompt.
  test("turned on, the device forgets that this account closed the prompt; turned off, it keeps that they did", async () => {
    const { fake, user } = await withPush(sends);
    const yuki = fake.accountId("yuki");
    rememberPushPromptClosed(yuki, true);
    const on = fake.script("PUT /push/subscription");
    const off = fake.script("DELETE /push/subscription");

    await user.click(toggle(push));
    await on.served();
    await waitFor(() => {
      expect(toggle(push).checked).toBe(true);
    });
    expect(promptClosed(yuki)).toBe(false);

    await user.click(toggle(push));
    await off.served();
    await waitFor(() => {
      expect(toggle(push).checked).toBe(false);
    });
    // A reader who turned it off themselves is not offered it again.
    expect(promptClosed(yuki)).toBe(true);
  });
});

describe("the switch says how turning Web Push on and off goes", () => {
  test("Saving… until the server took the subscription, then Saved", async () => {
    const { fake, user } = await withPush(sends);
    const sent = fake.script("PUT /push/subscription", { hold: "response" });
    expect(saidOnDevice()).toEqual([]);

    await user.click(toggle(push));
    await sent.arrived();

    expect(saidOnDevice()).toEqual(["Saving…"]);

    sent.release();
    await waitFor(() => {
      expect(saidOnDevice()).toEqual(["Saved"]);
    });
    expect(toggle(push).checked).toBe(true);
  });

  test("a browser's question closed without an answer: it says so, stays off and the reader's to turn on, and nothing subscribes", async () => {
    const drawn = await withPush(sends, { answer: "default" });
    const { user, seen } = drawn;

    await user.click(toggle(push));

    await waitFor(() => {
      expect(saidOnDevice()).toEqual([
        "The browser's question closed without an answer. Turn this on to be asked again.",
      ]);
    });
    expect(toggle(push).checked).toBe(false);
    expect(toggle(push).disabled).toBe(false);
    expect(seen).toMatchObject({ asked: 1, subscribed: 0 });
    expect(writes(drawn)).toEqual([]);

    // Turned on again, the browser asks again.
    await user.click(toggle(push));
    await waitFor(() => {
      expect(seen.asked).toBe(2);
    });
  });

  /** The reader turned the switch on and refused the browser's question; the screen says so. */
  async function refusedTheQuestion() {
    const drawn = await withPush(sends, { answer: "denied" });
    await drawn.user.click(toggle(push));
    await waitFor(() => {
      expect(saidOnDevice()).toEqual([blocked]);
    });
    return drawn;
  }

  test("a browser's question the reader refuses: it says that the browser blocks the site, stays off, and nothing subscribes", async () => {
    const drawn = await refusedTheQuestion();

    expect(toggle(push).checked).toBe(false);
    expect(drawn.seen).toMatchObject({ asked: 1, subscribed: 0 });
    expect(writes(drawn)).toEqual([]);
  });

  // The browser blocks the site from the refusal on, so the switch cannot be turned on any more, as on a device where
  // the site was blocked before the screen opened.
  test("once the reader refused the browser's question, the switch is disabled", async () => {
    await refusedTheQuestion();

    expect(toggle(push).disabled).toBe(true);
  });

  test("a subscription the browser fails to make: it says so and stays off, and turned on again it subscribes", async () => {
    const { fake, user, seen } = await withPush(sends, { subscribe: "fails-once" });
    const sent = fake.script("PUT /push/subscription");

    await user.click(toggle(push));

    await waitFor(() => {
      expect(saidOnDevice()).toEqual([notTurnedOn]);
    });
    expect(toggle(push).checked).toBe(false);
    expect(toggle(push).disabled).toBe(false);
    expect(sent.seen).toHaveLength(0);

    await user.click(toggle(push));

    await sent.served();
    await waitFor(() => {
      expect(columnsOf(people)).toContain("Push");
    });
    expect(toggle(push).checked).toBe(true);
    expect(seen.subscribed).toBe(2);
  });

  test("a subscription the server fails to take: it says so, and the switch is back off", async () => {
    const { fake, user } = await withPush(sends);
    fake.script("PUT /push/subscription", { fail: 500 });

    await user.click(toggle(push));

    await waitFor(() => {
      expect(saidOnDevice()).toEqual([notTurnedOn]);
    });
    expect(toggle(push).checked).toBe(false);
    expect(columnsOf(people)).not.toContain("Push");
  });

  test("a subscription the server fails to delete: the switch is back on, says so, and Try again deletes it", async () => {
    const { fake, user } = await withPush(subscribed, { permission: "granted" });
    const sent = fake.script("DELETE /push/subscription");
    const failing = fake.script("DELETE /push/subscription", { times: 1, fail: 500 });

    await user.click(toggle(push));

    await waitFor(() => {
      expect(saidOnDevice().join()).toMatch(/^This could not be saved\.\s*Try again$/);
    });
    expect(toggle(push).checked).toBe(true);
    expect(columnsOf(people)).toContain("Push");
    failing.end();

    await user.click(section(device).getByRole("button", { name: "Try again" }));

    await waitFor(() => {
      expect(columnsOf(people)).not.toContain("Push");
    });
    expect(toggle(push).checked).toBe(false);
    expect(sent.seen).toHaveLength(2);
  });
});

describe("where Web Push cannot be turned on, the switch is off and disabled and says why", () => {
  const cases: [string, FakeOptions, PushBrowser, string][] = [
    [
      "on a server that sends no Web Push",
      { settings: sends.settings },
      {},
      "This server does not send push notifications.",
    ],
    [
      "in a browser without the Push API",
      sends,
      { noPushApi: true },
      "This browser cannot receive push notifications.",
    ],
    ["where the site's notifications are blocked", sends, { permission: "denied" }, blocked],
  ];

  for (const [where, options, browser, why] of cases)
    test(where, async () => {
      const drawn = await withPush(options, browser);

      expect(toggle(push).checked).toBe(false);
      expect(toggle(push).disabled).toBe(true);
      expect(describedBy(toggle(push))).toEqual([why]);

      await drawn.user.click(toggle(push));

      expect(drawn.seen).toEqual({ asked: 0, subscribed: 0, played: 0 });
      expect(writes(drawn)).toEqual([]);
      expect(columnsOf(people)).not.toContain("Push");
    });
});

describe("the tables' Push column, while this device has a subscription", () => {
  /** The kinds the subscription is for once the server took the next change. */
  async function alertsAfter(drawn: Drawn, change: () => Promise<void>) {
    const sent = drawn.fake.script("PUT /push/subscription");
    await change();
    const { body } = await sent.served();
    expect(sent.seen).toHaveLength(1);
    return [...body.alerts].sort();
  }

  test("shows the kinds the subscription is for, and a cell adds its kind to them", async () => {
    const drawn = await withPush(subscribed, { permission: "granted" });
    expect(stateOf(cell(people, "Mentions", "Push"))).toBe("on");
    expect(stateOf(cell(people, "New followers", "Push"))).toBe("off");
    expect(stateOf(cell(people, "All", "Push"))).toBe("mixed");

    expect(await alertsAfter(drawn, () => drawn.user.click(cell(people, "New followers", "Push")))).toEqual([
      "follow",
      "mention",
    ]);

    await waitFor(() => {
      expect(stateOf(cell(people, "New followers", "Push"))).toBe("on");
    });
    expect(writes(drawn)).toEqual(["PUT /push/subscription"]);
  });

  test("a cell takes its kind away from them", async () => {
    const drawn = await withPush(subscribed, { permission: "granted" });

    expect(await alertsAfter(drawn, () => drawn.user.click(cell(people, "Mentions", "Push")))).toEqual([]);

    await waitFor(() => {
      expect(stateOf(cell(people, "Mentions", "Push"))).toBe("off");
    });
    // The device still has its subscription: the switch stays on, and the column stays.
    expect(toggle(push).checked).toBe(true);
    expect(columnsOf(people)).toContain("Push");
  });

  test("a row of several kinds of notification turns them all on", async () => {
    const drawn = await withPush(subscribed, { permission: "granted" });

    expect(await alertsAfter(drawn, () => drawn.user.click(cell(people, "Voice chats", "Push")))).toEqual([
      "mention",
      "voice_room_invited",
      "voice_room_started",
    ]);
  });

  test("the first row turns every kind of its table on in one request, and keeps the other table's", async () => {
    const drawn = await withPush(subscribed, { permission: "granted" });
    const ordinary = kindsOf(memberNotificationRows.filter((row) => row.section === "ordinary"));

    expect(await alertsAfter(drawn, () => drawn.user.click(cell(people, "All", "Push")))).toEqual(ordinary);

    await waitFor(() => {
      expect(stateOf(cell(people, "All", "Push"))).toBe("on");
    });
    expect(stateOf(cell(server, "All", "Push"))).toBe("off");
  });
});

describe("the notification sound", () => {
  test("is off on a new device, says what it does, and the tables have no Sound", async () => {
    await readSettings(sends);

    expect(toggle(sound).checked).toBe(false);
    expect(toggle(sound).disabled).toBe(false);
    expect(describedBy(toggle(sound))).toEqual([
      "A sound plays on this device when a notification arrives while the app is open. While this is on, choose which kinds in the “Sound” column below.",
    ]);
    expect(columnsOf(people)).not.toContain("Sound");
  });

  test("turned on, the tables gain Sound with no kind on yet, and the server is told nothing", async () => {
    const drawn = await withPush(sends);

    await drawn.user.click(toggle(sound));

    await waitFor(() => {
      expect(columnsOf(people)).toContain("Sound");
    });
    expect(toggle(sound).checked).toBe(true);
    expect(columnsOf(server)).toContain("Sound");
    expect(stateOf(cell(people, "All", "Sound"))).toBe("off");
    expect(stateOf(cell(people, "Mentions", "Sound"))).toBe("off");
    expect(stateOf(cell(server, "All", "Sound"))).toBe("off");
    expect(writes(drawn)).toEqual([]);
  });

  test("a cell of Sound turns its kind's sound on, the first row a whole table's, and the server is told nothing", async () => {
    const drawn = await withPush(sends, {});
    const { user } = drawn;
    await user.click(toggle(sound));
    await waitFor(() => {
      expect(columnsOf(people)).toContain("Sound");
    });

    await user.click(cell(people, "Mentions", "Sound"));

    await waitFor(() => {
      expect(stateOf(cell(people, "Mentions", "Sound"))).toBe("on");
    });
    expect(stateOf(cell(people, "All", "Sound"))).toBe("mixed");
    expect(stateOf(cell(people, "New followers", "Sound"))).toBe("off");

    await user.click(cell(people, "All", "Sound"));

    await waitFor(() => {
      expect(stateOf(cell(people, "All", "Sound"))).toBe("on");
    });
    expect(stateOf(cell(people, "New followers", "Sound"))).toBe("on");
    expect(stateOf(cell(people, "New posts", "Sound"))).toBe("on");
    // The other table's kinds are its own.
    expect(stateOf(cell(server, "All", "Sound"))).toBe("off");
    expect(writes(drawn)).toEqual([]);
  });

  test("is kept on this device: the entry opened again has the sound on, for the kinds chosen", async () => {
    const { user } = await withPush(sends);
    await user.click(toggle(sound));
    await waitFor(() => {
      expect(columnsOf(people)).toContain("Sound");
    });
    await user.click(cell(people, "Mentions", "Sound"));
    await user.click(cell(server, "Announcements", "Sound"));
    await waitFor(() => {
      expect(stateOf(cell(server, "Announcements", "Sound"))).toBe("on");
    });

    // Another load of the app, on a server that knows nothing of it.
    await drawnOn(fakeData(sends));
    await screen.findAllByRole("table");

    expect(toggle(sound).checked).toBe(true);
    expect(stateOf(cell(people, "Mentions", "Sound"))).toBe("on");
    expect(stateOf(cell(people, "New followers", "Sound"))).toBe("off");
    expect(stateOf(cell(server, "Announcements", "Sound"))).toBe("on");
  });

  test("turned off, the tables lose Sound", async () => {
    const { user } = await readSettings(sends, { sounding: ["mention"] });
    expect(toggle(sound).checked).toBe(true);
    expect(stateOf(cell(people, "Mentions", "Sound"))).toBe("on");

    await user.click(toggle(sound));

    await waitFor(() => {
      expect(columnsOf(people)).not.toContain("Sound");
    });
    expect(toggle(sound).checked).toBe(false);
  });

  for (const [when, sounding] of [
    ["while the sound is off", undefined],
    ["while the sound is on", ["mention"]],
  ] as const)
    test(`Play the sound plays it once ${when}, and the server is told nothing`, async () => {
      const drawn = await withPush(sends, {}, sounding);
      // Drawing the screen plays nothing.
      expect(drawn.seen.played).toBe(0);

      await drawn.user.click(section(device).getByRole("button", { name: "Play the sound" }));

      await waitFor(() => {
        expect(drawn.seen.played).toBe(1);
      });
      expect(saidOnDevice()).toEqual([]);
      expect(writes(drawn)).toEqual([]);
      // Trying the sound changes no setting.
      expect(toggle(sound).checked).toBe(sounding !== undefined);
    });

  test("where the browser does not let the sound play, Play the sound says so, and how to allow it", async () => {
    const { user, seen } = await withPush(sends, { audio: "blocked" });

    await user.click(section(device).getByRole("button", { name: "Play the sound" }));

    await waitFor(() => {
      expect(saidOnDevice()).toEqual([
        "The browser did not play the sound. Allow sound for this site in the browser's settings, then try again.",
      ]);
    });
    expect(seen.played).toBe(0);
  });
});
