// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { staticToken, type components } from "@mycroblog/mykit";
import { AlreadyRunningError, Bot, NotBotError, ScopeError } from "./bot.js";
import { FileStore } from "./filestore.js";
import { MemoryStore } from "./store.js";
import { account, delay, eventually, FakeServer, id, mention, processed, start, status } from "./test/fake.js";

type Notification = components["schemas"]["Notification"];

describe("receiving", () => {
  // The mentions that arrive while the event stream is down are caught up after the reconnection, and each mention
  // reaches the handler once, although both the stream and the catch-up bring some of them.
  test("the stream catches up after reconnecting, and each mention is handled once", async () => {
    const f = new FakeServer();
    const alice = account(1, "alice");
    const a = mention(10, alice, "first");
    const b = mention(11, alice, "while the stream was down");
    const c = mention(12, alice, "after the reconnection");
    const seen = new Map<string, number>();
    let handledA!: () => void;
    const aHandled = new Promise<void>((resolve) => {
      handledA = resolve;
    });
    f.stream = async (conn) => {
      if (conn.n === 1) {
        // Wait for the catch-up of the first connection to read the (empty) list, so that it misses b.
        await eventually("the first catch-up", () => f.listCalls >= 1);
        conn.send("notification", a);
        await aHandled;
        // The stream drops; b arrives before the next connection.
        f.add(a);
        f.add(b);
        conn.close();
        return;
      }
      // The next connection brings c on both paths, and a again.
      f.add(c);
      conn.send("notification", c);
      conn.send("notification", a);
    };
    const options = f.options({ mode: "streaming" });
    const bot = new Bot(options);
    bot.onMention((ctx) => {
      const n = ctx.notification?.id ?? "";
      seen.set(n, (seen.get(n) ?? 0) + 1);
      if (n === a.id) handledA();
    });
    const stop = start(bot);
    await eventually("the three mentions processed", () => processed(options.store, a.id, b.id, c.id));
    // Give a duplicate the time to arrive.
    await delay(100);
    await stop();
    for (const n of [a, b, c]) expect(seen.get(n.id)).toBe(1);
    expect(f.streamCalls).toBeGreaterThanOrEqual(2);
    expect(await options.store.position()).toBe(c.id);
  });

  test("the catch-up stops at the position and at the age limit", async () => {
    const f = new FakeServer();
    const old: Notification = {
      ...mention(5, account(1, "alice"), "old"),
      created_at: new Date(Date.now() - 48 * 3600_000).toISOString(),
    };
    f.add(old);
    f.add(mention(6, account(1, "alice"), "before the position"));
    f.add(mention(7, account(1, "alice"), "at the position"));
    f.add(mention(8, account(1, "alice"), "new"));
    const options = f.options();
    await options.store.setPosition(id(7));
    const bot = new Bot(options);
    const handled: string[] = [];
    bot.onMention((ctx) => {
      handled.push(ctx.notification?.id ?? "");
    });
    const stop = start(bot);
    await eventually("the new mention processed", () => processed(options.store, id(8)));
    await delay(30);
    await stop();
    expect(handled).toEqual([id(8)]);

    // Without a position, the age limit stops it.
    const f2 = new FakeServer();
    f2.add(old);
    f2.add(mention(9, account(1, "alice"), "recent"));
    const options2 = f2.options();
    const bot2 = new Bot(options2);
    bot2.onMention(() => undefined);
    const stop2 = start(bot2);
    await eventually("the recent mention processed", () => processed(options2.store, id(9)));
    await stop2();
    expect(await options2.store.isProcessed(id(5))).toBe(false);
  });

  // ADR-0094 §3: a catch-up limit of 0 turns the catch-up off.
  test("catchUpLimit 0 turns the catch-up off", async () => {
    const f = new FakeServer();
    f.add(mention(10, account(1, "alice"), "missed"));
    const options = f.options({ mode: "streaming", catchUpLimit: 0 });
    const bot = new Bot(options);
    const handled: string[] = [];
    bot.onMention((ctx) => {
      handled.push(ctx.notification?.id ?? "");
    });
    const live = mention(11, account(1, "alice"), "live");
    f.stream = (conn) => {
      conn.send("notification", live);
    };
    const stop = start(bot);
    await eventually("the live mention processed", () => processed(options.store, live.id));
    await stop();
    expect(handled).toEqual([live.id]);
    expect(f.listCalls).toBe(0);
  });

  test("a second run is refused", async () => {
    const f = new FakeServer();
    const bot = new Bot(f.options());
    const stop = start(bot);
    expect(bot.run()).rejects.toBeInstanceOf(AlreadyRunningError);
    await stop();
  });
});

// Handling a notification again, after a restart that lost the record, sends the same Idempotency-Key, so that the
// server posts the reply once.
test("a reply sends the same Idempotency-Key when its notification is handled again", async () => {
  const f = new FakeServer();
  const n = mention(10, account(1, "alice"), "hello");
  f.add(n);
  for (let run = 0; run < 2; run++) {
    const options = f.options();
    const bot = new Bot(options);
    bot.onMention(async (ctx) => {
      await ctx.reply("one");
      await ctx.reply("two");
    });
    const stop = start(bot);
    await eventually("the mention processed", () => processed(options.store, n.id));
    await stop();
  }
  expect(f.posts.map((p) => p.key)).toEqual([
    `mybot:reply:${n.id}:1`,
    `mybot:reply:${n.id}:2`,
    `mybot:reply:${n.id}:1`,
    `mybot:reply:${n.id}:2`,
  ]);
});

describe("manners", () => {
  // No handler for the bot itself, for a bot, or for a sender past the reply limit; replies no wider than their post,
  // mentioning the sender alone.
  test("the defaults", async () => {
    const f = new FakeServer();
    const self = account(0, "bot", true);
    const otherBot = account(2, "robot@remote.example", true);
    const alice = account(1, "alice");
    const dave = account(3, "dave@remote.example");
    const ns: Notification[] = [mention(10, self, "me"), mention(11, otherBot, "a bot")];
    // Six mentions of alice: the sixth is past 5 replies in 10 minutes.
    for (let i = 0; i < 6; i++) ns.push(mention(20 + i, alice, "again"));
    const visibilities = ["private", "direct", "local", "unlisted"] as const;
    for (const [i, v] of visibilities.entries()) ns.push(mention(30 + i, dave, "visibility", v));
    for (const n of ns) f.add(n);

    const options = f.options();
    const bot = new Bot(options);
    const handled: string[] = [];
    bot.onMention(async (ctx) => {
      handled.push(ctx.notification?.id ?? "");
      await ctx.reply("hi");
    });
    const stop = start(bot);
    await eventually("every notification processed", () => processed(options.store, ...ns.map((n) => n.id)));
    await stop();

    for (const skipped of [id(10), id(11), id(25)]) expect(handled).not.toContain(skipped);
    expect(handled).toHaveLength(5 + visibilities.length);
    const got = new Map<string, string>();
    for (const p of f.posts) {
      const to = ns.find((n) => n.status?.id === p.body.in_reply_to_id)?.account;
      expect(p.body.text).toBe(`@${to?.acct ?? "?"} hi`);
      got.set(p.body.in_reply_to_id ?? "", p.body.visibility);
    }
    expect(got.get(id(520))).toBe("unlisted");
    for (const [i, v] of visibilities.entries()) expect(got.get(id(530 + i))).toBe(v);
  });

  test("respondToBots and disableReplyLimits lift those that can be lifted", async () => {
    const f = new FakeServer();
    const robot = account(2, "robot", true);
    for (let i = 0; i < 7; i++) f.add(mention(10 + i, robot, "again"));
    const options = f.options({ respondToBots: true, disableReplyLimits: true });
    const bot = new Bot(options);
    let count = 0;
    bot.onMention(async (ctx) => {
      count++;
      await ctx.reply("hi");
    });
    const stop = start(bot);
    await eventually("every notification processed", () => processed(options.store, id(10), id(16)));
    await stop();
    expect(count).toBe(7);
  });
});

// A rotated refresh token is saved in the Store, and the next start uses it rather than the one of the options.
test("the rotated refresh token is saved and used by the next start", async () => {
  const f = new FakeServer();
  f.tokens.set("rt1", "rt2");
  const dir = await mkdtemp(join(tmpdir(), "mybot-"));
  try {
    const path = join(dir, "state.json");
    for (const want of ["rt2", "rt2'"]) {
      const store = new FileStore(path);
      const { tokenSource: _, ...options } = f.options({ store });
      const bot = new Bot({ ...options, oauth: { clientId: "client", clientSecret: "secret" }, refreshToken: "rt1" });
      const stop = start(bot);
      await eventually("the token renewed", async () => (await store.refreshToken()) === want);
      await stop();
      expect(await new FileStore(path).refreshToken()).toBe(want);
    }
    expect(f.refreshTokens).toEqual(["rt1", "rt2"]);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

describe("bot flag", () => {
  test("an account without it stops the bot", async () => {
    const f = new FakeServer();
    f.me = { ...f.me, bot: false };
    expect(new Bot(f.options()).run()).rejects.toBeInstanceOf(NotBotError);
    await delay(10);
    expect(f.botUpdates).toEqual([]);
  });

  test("setBotFlag sets it", async () => {
    const f = new FakeServer();
    f.me = { ...f.me, bot: false };
    const bot = new Bot(f.options({ setBotFlag: true }));
    const stop = start(bot);
    await eventually("the flag set", () => f.botUpdates.length > 0);
    await stop();
    expect(f.botUpdates).toEqual([true]);
    expect(f.me.bot).toBe(true);
  });

  test("allowNonBot runs without it", async () => {
    const f = new FakeServer();
    f.me = { ...f.me, bot: false };
    const bot = new Bot(f.options({ allowNonBot: true }));
    const stop = start(bot);
    await eventually("the notifications read", () => f.listCalls > 0);
    await stop();
    expect(f.botUpdates).toEqual([]);
  });
});

// A token without the scopes the bot needs stops it, naming them.
test("missing scopes stop the bot", async () => {
  const f = new FakeServer();
  f.scope = "read:accounts read:notifications follow";
  f.tokens.set("rt1", "rt2");
  const { tokenSource: _, ...options } = f.options({ setBotFlag: true });
  const error: unknown = await new Bot({ ...options, oauth: { clientId: "client" }, refreshToken: "rt1" })
    .run()
    .catch((e: unknown) => e);
  expect(error).toBeInstanceOf(ScopeError);
  expect((error as ScopeError).missing).toEqual(["read:statuses", "write:statuses", "write:accounts"]);
});

// Commands match the first word after the bot's mention without case; help lists them; other mentions fall to
// onMention, and other notifications to onNotification.
test("routing", async () => {
  const f = new FakeServer();
  f.add(mention(10, account(1, "alice"), "PING now please"));
  f.add(mention(11, account(2, "bob"), "help"));
  f.add(mention(12, account(3, "carol"), "just saying hi"));
  f.add({ id: id(13), type: "follow", created_at: new Date().toISOString(), account: account(4, "dave") });
  f.add({ id: id(14), type: "poll_closed", created_at: new Date().toISOString() });
  const options = f.options();
  const bot = new Bot(options);
  const got = new Map<string, string>();
  const record = (kind: string) => (ctx: { notification: Notification | undefined; command: string; args: string }) => {
    got.set(ctx.notification?.id ?? "", `${kind}:${ctx.command}:${ctx.args}`);
  };
  bot.onCommand("PING", "answer pong", record("command"));
  bot.onMention(record("mention"));
  bot.onFollow(record("follow"));
  bot.onNotification(record("notification"));
  const stop = start(bot);
  await eventually("every notification processed", () =>
    processed(options.store, id(10), id(11), id(12), id(13), id(14)),
  );
  await stop();
  expect(Object.fromEntries(got)).toEqual({
    [id(10)]: "command:ping:now please",
    [id(12)]: "mention::",
    [id(13)]: "follow::",
    [id(14)]: "notification::",
  });
  expect(f.posts.map((p) => p.body.text)).toEqual(["@bob help: list the commands\nping: answer pong"]);
  expect(bot.commands().map((c) => c.name)).toEqual(["help", "ping"]);
});

// A handler that throws is recovered, its notification counts as processed, and the bot goes on.
test("a handler that throws does not stop the bot", async () => {
  const f = new FakeServer();
  f.add(mention(10, account(1, "alice"), "boom"));
  f.add(mention(11, account(1, "alice"), "fine"));
  const options = f.options();
  const bot = new Bot(options);
  bot.onMention(async (ctx) => {
    if (ctx.text === "@bot boom") throw new Error("boom");
    await ctx.react("👍");
  });
  const stop = start(bot);
  await eventually("both processed", () => processed(options.store, id(10), id(11)));
  await stop();
  expect(f.reactions).toEqual([`${id(511)} 👍`]);
});

// A handler still running after the grace period is aborted, and its notification stays unprocessed for the next start.
test("the shutdown aborts the handlers after the grace period", async () => {
  const f = new FakeServer();
  f.add(mention(10, account(1, "alice"), "slow"));
  const options = f.options({ shutdownGrace: 50 });
  const bot = new Bot(options);
  let started!: () => void;
  const handlerStarted = new Promise<void>((resolve) => {
    started = resolve;
  });
  let aborted = false;
  bot.onMention(async (ctx) => {
    started();
    await new Promise<void>((resolve) => {
      ctx.signal.addEventListener(
        "abort",
        () => {
          resolve();
        },
        { once: true },
      );
    });
    aborted = true;
  });
  const stop = start(bot);
  await handlerStarted;
  await stop();
  await eventually("the handler aborted", () => aborted);
  await delay(20);
  expect(await options.store.isProcessed(id(10))).toBe(false);
  expect(await options.store.position()).toBeNull();
});

test("onStatus receives the home timeline of the stream, but the bot's own posts", async () => {
  const f = new FakeServer();
  const alice = account(1, "alice");
  const theirs = status(510, alice, "<p>a post</p>", "public");
  const mine = status(511, f.me, "<p>my post</p>", "public");
  f.stream = (conn) => {
    conn.send("update", mine);
    conn.send("update", theirs);
  };
  const bot = new Bot(f.options({ mode: "streaming" }));
  const seen: string[] = [];
  bot.onStatus((ctx) => {
    seen.push(ctx.status?.id ?? "");
  });
  const stop = start(bot);
  await eventually("the post handled", () => seen.length > 0);
  await delay(20);
  await stop();
  expect(seen).toEqual([theirs.id]);
});

test("the requests carry the bot's User-Agent", async () => {
  const f = new FakeServer();
  const bot = new Bot(f.options({ userAgent: "echobot/1.0" }));
  const stop = start(bot);
  await eventually("the notifications read", () => f.listCalls > 0);
  await stop();
  expect(new Set(f.userAgents)).toEqual(new Set(["echobot/1.0 mybot-ts"]));
});

test("the options need a token", () => {
  expect(() => new Bot({ server: "https://example.com" })).toThrow(/tokenSource/);
  expect(() => new Bot({ server: "", tokenSource: staticToken("t"), store: new MemoryStore() })).toThrow(/server/);
});
