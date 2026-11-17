// SPDX-License-Identifier: AGPL-3.0-only

// A post's own screen: the post, then its replies. That a reply stays in it is the E2E tests'
// (web/e2e/specs/thread.spec.ts).

import { expect, test } from "bun:test";
import { within } from "@testing-library/react";
import { main, renderApp } from "./test/app";

test("a post opens with its replies in the order they were written", async () => {
  const app = await renderApp("/", { viewer: "sam" });
  await app.navigate(app.fake.postPath("yuki-ridge"));

  expect(await main().findByRole("heading", { level: 1, name: "Post" })).toBeDefined();
  const thread = await main().findByRole("feed", { name: "Thread" });
  const said = [
    "今朝の尾根。三時に起きた甲斐がありました",
    "That light! Which trail is this?",
    "北側の尾根道です。駐車場から二時間くらい。",
    "Adding it to my list for the spring.",
    "きれい！次の集まりのポスターに使わせてほしいです",
  ];
  const posts = within(thread).getAllByRole("article");
  expect(posts).toHaveLength(said.length);
  for (const [at, post] of posts.entries()) expect(post.textContent).toContain(said[at] ?? "");
});
