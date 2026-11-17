// SPDX-License-Identifier: AGPL-3.0-only

// A post's own screen, at the post's address: the post of the address, then its replies. The replies and their order
// are the thread feature's (web/app-features/src/thread/thread.dom.test.tsx); that a reply stays in it is the E2E tests'
// (web/e2e/specs/thread.spec.ts).

import { expect, test } from "bun:test";
import { main, renderApp } from "./test/app";
import { postOf } from "./test/posts";

test("the address of a post opens its screen on that post", async () => {
  const app = await renderApp("/", { viewer: "sam" });
  await app.navigate(app.fake.postPath("yuki-ridge"));

  expect(await main().findByRole("heading", { level: 1, name: "Post" })).toBeDefined();
  const thread = await main().findByRole("feed", { name: "Thread" });
  expect(postOf(thread, "Yuki 雪", "今朝の尾根。三時に起きた甲斐がありました")).toBeDefined();
});
