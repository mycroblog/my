// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { Bot } from "../bot.js";
import { Clock, Server } from "./index.js";

/** Runs bot until the returned stop is called. */
function start(bot: Bot): () => Promise<void> {
  const controller = new AbortController();
  const done = bot.run({ signal: controller.signal });
  done.catch(() => undefined);
  return async () => {
    controller.abort();
    await done;
  };
}

describe("Server", () => {
  test("answers 401 without an access token, and 404 for what it does not serve", async () => {
    const srv = new Server();
    expect((await srv.fetch(new Request(`${srv.url}/api/v1/accounts/verify_credentials`))).status).toBe(401);
    const missing = await srv.fetch(new Request(`${srv.url}/api/v1/timelines/home`, { headers: { Authorization: "Bearer x" } }));
    expect(missing.status).toBe(404);
    expect(missing.headers.get("Content-Type")).toBe("application/problem+json");
  });

  test("posts once per Idempotency-Key", async () => {
    const srv = new Server();
    const post = () =>
      srv.fetch(
        new Request(`${srv.url}/api/v1/statuses`, {
          method: "POST",
          headers: { Authorization: "Bearer x", "Idempotency-Key": "k", "Content-Type": "application/json" },
          body: JSON.stringify({ text: "hi", visibility: "unlisted" }),
        }),
      );
    const first = (await (await post()).json()) as { id: string };
    const second = (await (await post()).json()) as { id: string };
    expect(second.id).toBe(first.id);
    expect(srv.posted()).toHaveLength(1);
  });

  test("pages the notifications newest first by their cursor", async () => {
    const srv = new Server();
    const alice = srv.account("alice");
    const ids = [1, 2, 3].map(() => srv.injectMissed(srv.notification("follow", alice)).id);
    const page = async (cursor?: string) =>
      (await (
        await srv.fetch(
          new Request(`${srv.url}/api/v1/notifications?limit=2${cursor === undefined ? "" : `&cursor=${cursor}`}`, { headers: { Authorization: "Bearer x" } }),
        )
      ).json()) as { items: { id: string }[]; next_cursor?: string };
    const first = await page();
    expect(first.items.map((n) => n.id)).toEqual(ids.slice(1).reverse());
    const second = await page(first.next_cursor);
    expect(second.items.map((n) => n.id)).toEqual(ids.slice(0, 1));
    expect(second.next_cursor).toBeUndefined();
  });

  test("a bot catches up the notifications of a disconnection once, and its helpers reach the server", async () => {
    const srv = new Server();
    const options = srv.options();
    const bot = new Bot(options);
    const seen: string[] = [];
    bot.onMention(async (ctx) => {
      seen.push(ctx.text);
      await ctx.react("👍");
      await ctx.favourite();
      await ctx.follow();
    });
    const stop = start(bot);
    const alice = srv.account("alice");
    await srv.waitStreams(1);
    srv.inject(srv.mention(alice, "first", "public"));
    while (seen.length < 1) await new Promise((r) => setTimeout(r, 5));

    srv.disconnect();
    const missed = srv.inject(srv.mention(alice, "while down", "public"));
    srv.reconnect();
    while (!(await options.store.isProcessed(missed.id))) await new Promise((r) => setTimeout(r, 5));
    await stop();
    srv.close();

    expect(seen).toEqual(["@bot first", "@bot while down"]);
    expect(srv.reactions()).toHaveLength(2);
    expect(srv.favourites()).toHaveLength(2);
    expect(srv.follows()).toEqual([alice.id]);
  });
});

describe("Clock", () => {
  test("wakes the sleepers whose time came, and drops those whose signal aborts", async () => {
    const clock = new Clock(0);
    const woke: number[] = [];
    const controller = new AbortController();
    const a = clock.sleep(1000, new AbortController().signal).then(() => woke.push(clock.now()));
    const b = clock.sleep(5000, controller.signal).catch(() => woke.push(-1));
    await clock.waitForSleepers(2);
    clock.advance(1000);
    await a;
    controller.abort();
    await b;
    expect(woke).toEqual([1000, -1]);
    expect(clock.sleepers).toBe(0);
    expect(() => {
      clock.set(0);
    }).toThrow();
  });

  test("starts at the current time, or at the option now of the server", () => {
    const before = Date.now() - 1000;
    const now = new Server().clock.now();
    expect(now).toBeGreaterThanOrEqual(before);
    expect(now).toBeLessThanOrEqual(Date.now());

    // A time in the past stays settable on any day.
    const at = Date.parse("2001-02-03T04:05:06Z");
    const fixed = new Server({ now: at });
    expect(fixed.clock.now()).toBe(at);
    expect(fixed.me.created_at).toBe(new Date(at).toISOString());
    fixed.clock.set(at + 3_600_000);
    expect(fixed.clock.now()).toBe(at + 3_600_000);
  });
});
