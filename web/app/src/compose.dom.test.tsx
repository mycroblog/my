// SPDX-License-Identifier: AGPL-3.0-only

// A post sent from the composer closes it and shows in the screen under it. That the composer is not left while a post
// is sent is the composer's (web/app-features/src/compose/drafts.dom.test.tsx).

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp } from "./test/app";
import { newPost, paste } from "./test/composer";
import { feedOf, postsLike } from "./test/posts";

test("a post sent from the composer closes it and shows in the home timeline", async () => {
  const text = "In-flight check: the ferry leaves at noon.";
  const app = await renderApp("/", { viewer: "yuki" });
  const feed = await feedOf("Home timeline");
  const composer = await newPost(app);
  await paste(app, composer.text, text);
  const sent = app.fake.script("POST /statuses");

  await app.user.click(within(composer.dialog).getByRole("button", { name: "Post" }));

  await waitFor(() => {
    if (screen.queryByRole("dialog", { name: "New post" }) !== null)
      throw new Error("test: the composer is still open");
  });
  await waitFor(() => {
    expect(postsLike(feed, "Yuki 雪", text)).toHaveLength(1);
  });
  expect(sent.seen).toHaveLength(1);
});
