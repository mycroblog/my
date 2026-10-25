// SPDX-License-Identifier: MIT
import { expect, test } from "bun:test";
import { Server } from "@mycroblog/mybot/testing";
import { createBot } from "./bot.ts";

test("the scheduled bot greets at 9:00", async () => {
  const srv = new Server({ now: Date.parse("2026-10-26T08:59:00+09:00") });
  const options = srv.options();
  // The bot posted yesterday's greeting, so catchUp "latest" has nothing to run when it starts. A job that never ran
  // would post its newest past time, yesterday's 9:00, at once.
  await options.store.setLastRun("morning", Date.parse("2026-10-25T09:00:00+09:00"));
  const bot = createBot(options, "Asia/Tokyo");
  const controller = new AbortController();
  const done = bot.run({ signal: controller.signal });

  // The job and the pruning of the store sleep on the clock until the test moves it on to 9:00.
  await srv.clock.waitForSleepers(2);
  srv.clock.advance(60_000);
  const [post] = await srv.waitPosted(1);
  expect(post?.request.text).toBe("Good morning! Today is Monday, 26 October 2026.");
  expect(post?.request.visibility).toBe("unlisted");
  expect(post?.idempotencyKey).toBe("mybot:every:morning:2026-10-26T00:00:00Z");

  controller.abort();
  await done;
  srv.close();
});

test("a bot stopped for three mornings posts the newest of them once", async () => {
  const srv = new Server({ now: Date.parse("2026-10-26T12:00:00Z") });
  const options = srv.options();
  await options.store.setLastRun("morning", Date.parse("2026-10-23T09:00:00Z"));
  const bot = createBot(options);
  const controller = new AbortController();
  const done = bot.run({ signal: controller.signal });

  const [post] = await srv.waitPosted(1);
  expect(post?.request.text).toBe("Good morning! Today is Monday, 26 October 2026.");
  // Nothing else is due until the next morning.
  await srv.clock.waitForSleepers(2);
  expect(srv.posted()).toHaveLength(1);

  controller.abort();
  await done;
  srv.close();
});
