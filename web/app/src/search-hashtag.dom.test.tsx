// SPDX-License-Identifier: AGPL-3.0-only

// The search finds hashtags (FR-306): a hashtag found says how many posts it has, and opens its timeline. Which words
// find which hashtag is the server's tests' (TestTagSearchAPI); the accounts found are find.dom.test.tsx. The reader is
// Sam, on a server that holds one post with the hashtag.

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { within } from "@testing-library/react";
import { main, renderApp } from "./test/app";
import { feedOf, postOf } from "./test/posts";

test("the search finds a hashtag with the count of its posts, and the hashtag opens its timeline", async () => {
  const app = await renderApp("/search", {
    viewer: "sam",
    seed: {
      people: scenario.people,
      posts: [
        { key: "rye", author: "yuki", text: "Crumb shot of the rye. #crumbshot", visibility: "public", language: "en" },
      ],
    },
  });

  await app.user.type(main().getByRole("searchbox", { name: "Search" }), "crumbshot");
  const found = within(await main().findByRole("list", { name: "Hashtags" }));
  await app.user.click(await found.findByRole("link", { name: /^#crumbshot\s*1 post$/ }));

  expect(await main().findByRole("heading", { level: 1, name: "#crumbshot" })).toBeDefined();
  expect(app.path()).toBe("/tags/crumbshot");
  expect(postOf(await feedOf("Posts with #crumbshot"), "Yuki 雪", "Crumb shot of the rye.")).toBeDefined();
});
