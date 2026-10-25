// SPDX-License-Identifier: MIT
import { expect, test } from "bun:test";
import { Server } from "@mycroblog/mybot/testing";
import { createBot, withoutMentions } from "./bot.ts";

test("the echo bot says a mention again", async () => {
  const srv = new Server();
  const bot = createBot(srv.options());
  const controller = new AbortController();
  const done = bot.run({ signal: controller.signal });

  const alice = srv.account("alice");
  srv.inject(srv.mention(alice, "hello, world", "private"));
  srv.inject(srv.mention(alice, "how are you?", "public"));
  const posts = await srv.waitPosted(2);
  // The reply keeps the visibility of a private mention, and a public one gets an unlisted reply.
  expect(posts.map((p) => [p.request.text, p.request.visibility])).toEqual([
    ["@alice hello, world", "private"],
    ["@alice how are you?", "unlisted"],
  ]);

  controller.abort();
  await done;
  srv.close();
});

test("withoutMentions", () => {
  expect(withoutMentions("@bot hello")).toBe("hello");
  expect(withoutMentions("@bot @alice  hi there")).toBe("hi there");
  expect(withoutMentions("@bot")).toBe("");
  expect(withoutMentions("hello @bot")).toBe("hello @bot");
});
