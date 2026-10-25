// SPDX-License-Identifier: MIT
import { expect, test } from "bun:test";
import { Server } from "@mycroblog/mybot/testing";
import { createBot, diceUsage, parseDice } from "./bot.ts";

test("the command bot answers help, ping, and dice", async () => {
  const srv = new Server();
  // The dice of the test throw 1, 2, 3, ... in turn.
  let next = 0;
  const bot = createBot(srv.options(), (n) => (next++ % n) + 1);
  const controller = new AbortController();
  const done = bot.run({ signal: controller.signal });

  // One sender gets at most 5 replies in 10 minutes, so the cases have two.
  const alice = srv.account("alice");
  const bob = srv.account("bob");
  const cases = [
    [alice, "help", "@alice dice: throw dice, such as dice 2d6\nhelp: list the commands\nping: answer pong"],
    [alice, "PING", "@alice pong"],
    [alice, "dice 3d6", "@alice 🎲 1 + 2 + 3 = 6"],
    [bob, "dice", "@bob 🎲 4"],
    [bob, "dice 99d6", `@bob ${diceUsage}`],
    [bob, "hello", "@bob I do not know that command. Try help."],
  ] as const;
  for (const [i, [from, text, want]] of cases.entries()) {
    srv.inject(srv.mention(from, text, "public"));
    const posts = await srv.waitPosted(i + 1);
    expect(posts[i]?.request.text).toBe(want);
  }

  controller.abort();
  await done;
  srv.close();
});

test("parseDice", () => {
  expect(parseDice("")).toEqual({ n: 1, sides: 6 });
  expect(parseDice("2d6")).toEqual({ n: 2, sides: 6 });
  expect(parseDice("D20")).toEqual({ n: 1, sides: 20 });
  expect(parseDice("10d1000")).toEqual({ n: 10, sides: 1000 });
  for (const bad of ["11d6", "1d1", "0d6", "six", "2dx"]) expect(parseDice(bad)).toBeUndefined();
});
