// SPDX-License-Identifier: MIT
// The position of the catch-up and the order of the notifications: they mirror checkpoint_test.go of the Go library.
import { expect, test } from "bun:test";
import { Bot } from "./bot.js";
import { MemoryStore } from "./store.js";
import { account, delay, eventually, FakeServer, mention, processed, start } from "./test/fake.js";
import { Clock } from "./testing/clock.js";

/** The first wait before a catch-up or a Store call that failed is tried again. */
const retryFirstWait = 1000;

/** A bot of f in the streaming mode that records the IDs of the mentions it handles. */
function streamingBot(f: FakeServer, clock?: Clock): { bot: Bot; store: MemoryStore; handled: string[] } {
  const store = new MemoryStore();
  const bot = new Bot(
    f.options({ mode: "streaming", store, ...(clock === undefined ? {} : { now: clock.now, sleep: clock.sleep }) }),
  );
  const handled: string[] = [];
  bot.onMention((ctx) => {
    handled.push(ctx.notification?.id ?? "");
  });
  return { bot, store, handled };
}

// A notification handed out again while its handler runs stays pending: a newer one processed meanwhile does not
// move the position past it.
test("a duplicate hand-out keeps the notification pending", async () => {
  const f = new FakeServer();
  const older = mention(10, account(1, "alice"), "slow");
  const newer = mention(11, account(2, "bob"), "fast");
  f.add(older);
  let release!: () => void;
  const released = new Promise<void>((resolve) => {
    release = resolve;
  });
  let started = false;
  f.stream = async (conn) => {
    await eventually("the older mention running", () => started);
    // The stream brings it again while it runs, and a newer one.
    conn.send("notification", older);
    f.add(newer);
    conn.send("notification", newer);
  };
  const store = new MemoryStore();
  const bot = new Bot(f.options({ mode: "streaming", store }));
  const handled: string[] = [];
  bot.onMention(async (ctx) => {
    const n = ctx.notification?.id ?? "";
    if (n === older.id) {
      started = true;
      await released;
    }
    handled.push(n);
  });
  const stop = start(bot);
  await eventually("the newer mention processed", () => processed(store, newer.id));
  await delay(50);
  expect(await store.position()).toBeNull();
  release();
  await eventually("the position", async () => (await store.position()) === newer.id);
  await stop();
  expect(handled).toEqual([newer.id, older.id]);
});

// A failed catch-up keeps the position, and the notifications of the stream, until it succeeds: a newer notification
// of the stream never moves the position past those it has yet to find.
test("a failed catch-up holds the position", async () => {
  const f = new FakeServer();
  const alice = account(1, "alice");
  const missed = mention(10, alice, "while the bot was stopped");
  const live = mention(11, alice, "on the stream");
  f.add(missed);
  // The first catch-up fails.
  f.list = (n) => n > 1;
  let sent = false;
  f.stream = async (conn) => {
    await eventually("the failed catch-up", () => f.listCalls >= 1);
    f.add(live);
    conn.send("notification", live);
    sent = true;
  };
  const clock = new Clock(Date.now());
  const { bot, store, handled } = streamingBot(f, clock);
  const stop = start(bot);
  try {
    await eventually("the live mention sent", () => sent);
    await delay(100);
    expect(await store.position()).toBeNull();
    expect(handled).toEqual([]);
    // The pruning and the retry of the catch-up sleep.
    await clock.waitForSleepers(2);
    clock.advance(retryFirstWait);
    await eventually("both mentions processed", () => processed(store, missed.id, live.id));
    await eventually("the position", async () => (await store.position()) === live.id);
    expect(handled).toEqual([missed.id, live.id]);
  } finally {
    await stop();
  }
});

// The notifications of the stream that arrive while the catch-up reads the missed ones are handled after them: every
// notification in the order of time.
test("the stream waits for the catch-up", async () => {
  const f = new FakeServer();
  const alice = account(1, "alice");
  const missed = mention(10, alice, "while the bot was stopped");
  const live = mention(11, alice, "on the stream");
  f.add(missed);
  f.add(live);
  let sent!: () => void;
  const streamed = new Promise<void>((resolve) => {
    sent = resolve;
  });
  f.list = async (n) => {
    if (n === 1) {
      // The stream brings the newer notification before the catch-up has read the older one.
      await streamed;
      await delay(100);
    }
    return true;
  };
  f.stream = (conn) => {
    conn.send("notification", live);
    sent();
  };
  const { bot, store, handled } = streamingBot(f);
  const stop = start(bot);
  await eventually("both mentions processed", () => processed(store, missed.id, live.id));
  await delay(50);
  await stop();
  expect(handled).toEqual([missed.id, live.id]);
});

/** A MemoryStore whose isProcessed and markProcessed fail for the notifications in failRead and failMark, once each. */
class FlakyStore extends MemoryStore {
  readonly failRead: Set<string>;
  readonly failMark: Set<string>;

  constructor(failRead: string[], failMark: string[]) {
    super();
    this.failRead = new Set(failRead);
    this.failMark = new Set(failMark);
  }

  override isProcessed(id: string): Promise<boolean> {
    if (this.failRead.delete(id)) return Promise.reject(new Error("the store is down"));
    return super.isProcessed(id);
  }

  override markProcessed(id: string, at: number): Promise<void> {
    if (this.failMark.delete(id)) return Promise.reject(new Error("the store is down"));
    return super.markProcessed(id, at);
  }
}

// A notification whose Store calls fail is tried again, without waiting for the server to send it again, and holds
// the position until it is processed.
test("a Store failure is tried again", async () => {
  const f = new FakeServer();
  const read = mention(10, account(1, "alice"), "read fails");
  const mark = mention(11, account(2, "bob"), "mark fails");
  const fine = mention(12, account(3, "carol"), "fine");
  f.add(read);
  f.add(mark);
  f.add(fine);
  const store = new FlakyStore([read.id], [mark.id]);
  const clock = new Clock(Date.now());
  const bot = new Bot(f.options({ store, now: clock.now, sleep: clock.sleep }));
  const handled = new Map<string, number>();
  bot.onMention((ctx) => {
    const n = ctx.notification?.id ?? "";
    handled.set(n, (handled.get(n) ?? 0) + 1);
  });
  const stop = start(bot);
  try {
    await eventually("the newest mention processed", () => processed(store, fine.id));
    // The pruning, the polling, and the two retries sleep.
    await clock.waitForSleepers(4);
    expect(await store.position()).toBeNull();
    clock.advance(retryFirstWait);
    await eventually("every mention processed", () => processed(store, read.id, mark.id, fine.id));
    await eventually("the position", async () => (await store.position()) === fine.id);
  } finally {
    await stop();
  }
  for (const n of [read, mark, fine]) expect(handled.get(n.id)).toBe(1);
});

/** A MemoryStore whose save of older waits for the save of newer, or for wait ms when none comes. */
class PositionStore extends MemoryStore {
  readonly saves: string[] = [];
  #newerSaved!: () => void;
  readonly #newer: Promise<void>;
  readonly older: string;
  readonly newer: string;
  readonly wait: number;

  constructor(older: string, newer: string, wait: number) {
    super();
    this.older = older;
    this.newer = newer;
    this.wait = wait;
    this.#newer = new Promise<void>((resolve) => {
      this.#newerSaved = resolve;
    });
  }

  override async setPosition(id: string): Promise<void> {
    this.saves.push(id);
    if (id === this.older) await Promise.race([this.#newer, delay(this.wait)]);
    await super.setPosition(id);
    if (id === this.newer) this.#newerSaved();
  }
}

// Two notifications that finish at once move the position at once: the older position, saved slowly, never lands
// after the newer one.
test("the position never moves back", async () => {
  const f = new FakeServer();
  // Two senders: the mentions of one sender are handled one at a time.
  const older = mention(10, account(1, "alice"), "first");
  const newer = mention(11, account(2, "bob"), "second");
  const store = new PositionStore(older.id, newer.id, 200);
  let release!: () => void;
  const released = new Promise<void>((resolve) => {
    release = resolve;
  });
  const running = new Set<string>();
  f.stream = (conn) => {
    f.add(older);
    conn.send("notification", older);
    f.add(newer);
    conn.send("notification", newer);
  };
  const bot = new Bot(f.options({ mode: "streaming", store }));
  bot.onMention(async (ctx) => {
    running.add(ctx.notification?.id ?? "");
    await released;
  });
  const stop = start(bot);
  await eventually("both mentions running", () => running.size === 2);
  release();
  await eventually("both mentions processed", () => processed(store, older.id, newer.id));
  await eventually("both positions saved", () => store.saves.length === 2);
  // The older position was computed first, so the saves could race.
  expect(store.saves).toEqual([older.id, newer.id]);
  await eventually("the position", async () => (await store.position()) === newer.id);
  await delay(store.wait + 50);
  await stop();
  expect(await store.position()).toBe(newer.id);
});
