// SPDX-License-Identifier: AGPL-3.0-only

// While a post is sent, the composer is not left: Close and Escape leave it open with what was written, and it closes
// once the server has answered.

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp } from "./test/app";
import { heldBy, newPost, paste } from "./test/composer";
import { feedOf, postsLike } from "./test/posts";

test("while a post is sent, Close and Escape leave the composer open, and it closes once posted", async () => {
  const text = "In-flight check: the ferry leaves at noon.";
  const app = await renderApp("/", { viewer: "yuki" });
  const feed = await feedOf("Home timeline");
  const composer = await newPost(app);
  await paste(app, composer.text, text);
  const sent = app.fake.script("POST /statuses", { hold: "response" });
  const post = within(composer.dialog).getByRole("button", { name: "Post" });
  const close = within(composer.dialog).getByRole<HTMLButtonElement>("button", { name: "Close" });
  expect(close.disabled).toBe(false);

  await app.user.click(post);
  expect((await sent.arrived()).body).toMatchObject({ text });
  // Close stays in its place, disabled, so that it cannot be pressed; Post alone shows it is sending.
  expect(within(composer.dialog).getByRole("button", { name: "Close" })).toBe(close);
  expect(close.disabled).toBe(true);
  expect(post.getAttribute("aria-busy")).toBe("true");
  await app.user.keyboard("{Escape}");
  expect(screen.getByRole("dialog", { name: "New post" })).toBe(composer.dialog);
  expect(heldBy(composer.text)).toBe(text);

  sent.release();
  await waitFor(() => {
    if (screen.queryByRole("dialog", { name: "New post" }) !== null)
      throw new Error("test: the composer is still open");
  });
  await waitFor(() => {
    expect(postsLike(feed, "Yuki 雪", text)).toHaveLength(1);
  });
  expect(sent.seen).toHaveLength(1);
  // Nothing was discarded: no Undo is offered.
  expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
});
