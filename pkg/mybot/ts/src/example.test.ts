// SPDX-License-Identifier: MIT
// The examples of @mycroblog/mybot/testing for the authors of bots: they import the package as a user does.
import { expect, test } from "bun:test";
import { Bot, replyIdempotencyKey } from "@mycroblog/mybot";
import { Server } from "@mycroblog/mybot/testing";

// A test of a command: the fake server sends the bot a mention, and the test reads the reply that it posted.
test("example: a command", async () => {
  const srv = new Server();
  const bot = new Bot(srv.options());
  bot.onCommand("echo", "say the text again", async (ctx) => {
    await ctx.reply(ctx.args);
  });
  const controller = new AbortController();
  const done = bot.run({ signal: controller.signal });

  const alice = srv.account("alice");
  srv.inject(srv.mention(alice, "echo hello", "public"));
  const [post] = await srv.waitPosted(1);
  expect(post?.request.text).toBe("@alice hello");
  expect(post?.request.visibility).toBe("unlisted");
  expect(post?.idempotencyKey).toBe(replyIdempotencyKey(srv.notifications()[0]?.id ?? "", 1));

  controller.abort();
  await done;
  srv.close();
});

// A test of a scheduled job: the bot's clock is the server's, which the test moves on to the job's next time.
test("example: a scheduled job", async () => {
  const srv = new Server();
  srv.clock.set(Date.parse("2026-10-25T08:59:00Z"));
  const bot = new Bot(srv.options());
  bot.every(
    "0 9 * * *",
    async (ctx) => {
      await ctx.post("Good morning");
    },
    { name: "morning" },
  );
  const controller = new AbortController();
  const done = bot.run({ signal: controller.signal });

  // The job and the pruning of the store sleep on the clock.
  await srv.clock.waitForSleepers(2);
  srv.clock.advance(60_000);
  const [post] = await srv.waitPosted(1);
  expect(post?.request.text).toBe("Good morning");
  expect(post?.request.visibility).toBe("unlisted");
  expect(post?.idempotencyKey).toBe("mybot:every:morning:2026-10-25T09:00:00Z");

  controller.abort();
  await done;
  srv.close();
});
