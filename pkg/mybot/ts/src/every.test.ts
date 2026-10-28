// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { Bot } from "./bot.js";
import { type Context } from "./context.js";
import { everyIdempotencyKey, NotScheduledError, PostedAlreadyError } from "./every.js";
import { ScheduleError } from "./schedule.js";
import { Server } from "./testing/index.js";

const minute = 60_000;
const hour = 60 * minute;

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

describe("every", () => {
  test("refuses what the Go library refuses", () => {
    const bot = new Bot(new Server().options());
    const noop = () => undefined;
    expect(() => {
      bot.every("0 0 * * * *", noop);
    }).toThrow(ScheduleError);
    expect(() => {
      bot.every("@daily", noop);
    }).toThrow(ScheduleError);
    expect(() => {
      bot.every("0 9 * * *", noop, { timeZone: "Mars/Olympus" });
    }).toThrow(ScheduleError);
    expect(() => {
      bot.every("0 9 * * *", noop, { visibility: "direct" as "public" });
    }).toThrow(/visibility/);
    bot.every("0 9 * * *", noop);
    expect(() => {
      bot.every("0 9 * * *", noop);
    }).toThrow(/named/);
    bot.every("0 9 * * *", noop, { name: "another" });
  });

  test("the idempotency key names the job and the time in RFC 3339", () => {
    expect(everyIdempotencyKey("morning", Date.parse("2026-10-25T09:00:00.000Z"))).toBe(
      "mybot:every:morning:2026-10-25T09:00:00Z",
    );
  });

  test("runs at each time, posts once per run, and records the last run", async () => {
    const srv = new Server({ now: Date.parse("2026-10-25T08:59:00Z") });
    const options = srv.options();
    const bot = new Bot(options);
    const errors: unknown[] = [];
    let outside: unknown;
    bot.onMention(async (ctx) => {
      try {
        await ctx.post("not a job");
      } catch (error) {
        outside = error;
      }
    });
    bot.every(
      "0 * * * *",
      async (ctx: Context) => {
        await ctx.post(`hour ${new Date(ctx.scheduledAt ?? 0).toISOString()}`);
        try {
          await ctx.post("again");
        } catch (error) {
          errors.push(error);
        }
      },
      { name: "hourly", visibility: "public" },
    );
    const stop = start(bot);
    await srv.clock.waitForSleepers(2);
    srv.clock.advance(minute);
    await srv.waitPosted(1);
    await srv.clock.waitForSleepers(2);
    srv.clock.advance(hour);
    const posts = await srv.waitPosted(2);
    srv.inject(srv.mention(srv.account("alice"), "hello", "public"));
    await srv.waitStreams(1);
    for (let i = 0; i < 50 && outside === undefined; i++) await new Promise((r) => setTimeout(r, 5));
    await stop();
    srv.close();

    expect(posts.map((p) => p.idempotencyKey)).toEqual([
      "mybot:every:hourly:2026-10-25T09:00:00Z",
      "mybot:every:hourly:2026-10-25T10:00:00Z",
    ]);
    expect(posts.map((p) => p.request.visibility)).toEqual(["public", "public"]);
    expect(errors).toHaveLength(2);
    expect(errors[0]).toBeInstanceOf(PostedAlreadyError);
    expect(outside).toBeInstanceOf(NotScheduledError);
    expect(await options.store.lastRun("hourly")).toBe(Date.parse("2026-10-25T10:00:00Z"));
  });

  test('catchUp "latest" runs the newest missed time once, and "skip" none', async () => {
    const srv = new Server({ now: Date.parse("2026-10-25T12:30:00Z") });
    const options = srv.options();
    // The bot last ran the job two days ago: the runs of yesterday and today were missed.
    await options.store.setLastRun("latest", Date.parse("2026-10-23T09:00:00Z"));
    await options.store.setLastRun("skip", Date.parse("2026-10-23T09:00:00Z"));
    const bot = new Bot(options);
    const runs: { job: string; at: number | undefined }[] = [];
    bot.every(
      "0 9 * * *",
      async (ctx) => {
        runs.push({ job: ctx.job, at: ctx.scheduledAt });
        await ctx.post("caught up");
      },
      { name: "latest", catchUp: "latest" },
    );
    bot.every(
      "0 9 * * *",
      (ctx) => {
        runs.push({ job: ctx.job, at: ctx.scheduledAt });
      },
      { name: "skip" },
    );
    const stop = start(bot);
    await srv.waitPosted(1);
    // The two jobs and the pruning sleep: nothing else runs.
    await srv.clock.waitForSleepers(3);
    await stop();

    expect(runs).toEqual([{ job: "latest", at: Date.parse("2026-10-25T09:00:00Z") }]);
    expect(srv.posted().map((p) => p.idempotencyKey)).toEqual(["mybot:every:latest:2026-10-25T09:00:00Z"]);
    expect(await options.store.lastRun("latest")).toBe(Date.parse("2026-10-25T09:00:00Z"));
    expect(await options.store.lastRun("skip")).toBe(Date.parse("2026-10-23T09:00:00Z"));
  });

  test('catchUp "latest" runs a job that never ran at its newest past time', async () => {
    const srv = new Server({ now: Date.parse("2026-10-25T12:30:00Z") });
    const bot = new Bot(srv.options());
    bot.every(
      "0 9 * * *",
      async (ctx) => {
        await ctx.post("first");
      },
      { name: "new", catchUp: "latest" },
    );
    const stop = start(bot);
    const posts = await srv.waitPosted(1);
    await stop();
    expect(posts.map((p) => p.idempotencyKey)).toEqual(["mybot:every:new:2026-10-25T09:00:00Z"]);
  });
});

describe("runOnce", () => {
  test("handles the missed notifications and the due jobs once, and returns", async () => {
    const srv = new Server({ now: Date.parse("2026-10-25T12:30:00Z") });
    const options = srv.options({ catchUpLimit: 0 });
    await options.store.setLastRun("daily", Date.parse("2026-10-23T09:00:00Z"));
    const alice = srv.account("alice");
    srv.injectMissed({ ...srv.mention(alice, "ping", "public"), created_at: new Date(srv.clock.now()).toISOString() });

    const handled: string[] = [];
    const build = (opts: typeof options) => {
      const bot = new Bot(opts);
      bot.onCommand("ping", "answer pong", async (ctx) => {
        handled.push("ping");
        await ctx.reply("pong");
      });
      // catchUp "skip" (the default): runOnce runs the due time all the same.
      bot.every(
        "0 9 * * *",
        async (ctx) => {
          handled.push("daily");
          await ctx.post("daily");
        },
        { name: "daily" },
      );
      return bot;
    };

    // catchUpLimit 0 turns the catch-up of the notifications off: the job alone runs.
    await build(options).runOnce();
    expect(handled).toEqual(["daily"]);

    // With the catch-up on, the mention is handled; the job ran already.
    const again = { ...options, catchUpLimit: 24 * hour };
    await build(again).runOnce();
    expect(handled).toEqual(["daily", "ping"]);
    await build(again).runOnce();
    expect(handled).toEqual(["daily", "ping"]);

    expect(srv.posted().map((p) => p.request.text)).toEqual(["daily", "@alice pong"]);
    expect(srv.posted()[0]?.idempotencyKey).toBe("mybot:every:daily:2026-10-25T09:00:00Z");
  });

  test("returns when its signal aborts, leaving the unfinished job due", async () => {
    const srv = new Server({ now: Date.parse("2026-10-25T12:30:00Z") });
    const options = srv.options({ shutdownGrace: 10 });
    const bot = new Bot(options);
    let started!: () => void;
    const running = new Promise<void>((resolve) => {
      started = resolve;
    });
    bot.every(
      "0 9 * * *",
      (ctx) =>
        new Promise<void>((resolve) => {
          started();
          ctx.signal.addEventListener(
            "abort",
            () => {
              resolve();
            },
            { once: true },
          );
        }),
      { name: "slow" },
    );
    const controller = new AbortController();
    const done = bot.runOnce({ signal: controller.signal });
    await running;
    controller.abort();
    await done;
    expect(await options.store.lastRun("slow")).toBeNull();
  });
});
