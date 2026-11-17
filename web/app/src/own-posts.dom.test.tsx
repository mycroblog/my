// SPDX-License-Identifier: AGPL-3.0-only

// A person deletes and redrafts their own post from its More menu; the menu offers each reader what they may do to a
// post, the thread's mute to one with an account.

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp } from "./test/app";
import { composerOf, heldBy } from "./test/composer";
import { feedOf, openMore, postOf, postsLike } from "./test/posts";

/** The words of the items of `menu`, in their order. */
const items = (menu: HTMLElement) =>
  within(menu)
    .getAllByRole("menuitem")
    .map((item) => item.textContent);

const bread = "Day 3 of the sourdough experiment. The crumb is finally open enough to call it bread. #baking";

test("delete and redraft takes the post away and opens it as a new post", async () => {
  const app = await renderApp("/", { viewer: "sam" });
  const feed = await feedOf("Home timeline");
  const deleted = app.fake.script("DELETE /statuses/{id}");

  const menu = await openMore(app, postOf(feed, "Sam Rivera", "Day 3 of the sourdough"));
  await app.user.click(within(menu).getByRole("menuitem", { name: "Delete and redraft…" }));
  const confirm = await screen.findByRole("alertdialog", { name: "Delete and redraft this post?" });
  await app.user.click(within(confirm).getByRole("button", { name: "Delete and redraft" }));

  const composer = await composerOf("New post");
  expect(heldBy(composer.text)).toBe(bread);
  expect(deleted.seen).toHaveLength(1);
  expect(postsLike(feed, "Sam Rivera", "Day 3 of the sourdough")).toHaveLength(0);
});

test("the More menu of one's own post: its link, sharing, the bookmark, the thread's mute, edit, redraft and delete", async () => {
  const app = await renderApp("/", { viewer: "sam" });
  const menu = await openMore(app, postOf(await feedOf("Home timeline"), "Sam Rivera", "Day 3 of the sourdough"));

  expect(items(menu)).toEqual([
    "Copy link",
    "Share",
    "Bookmark",
    "Mute thread",
    "Edit",
    "Delete and redraft…",
    "Delete…",
  ]);
});

test("the More menu of one's own post with a poll: no edit, which the server refuses", async () => {
  const app = await renderApp("/", { viewer: "sam" });
  const menu = await openMore(app, postOf(await feedOf("Home timeline"), "Sam Rivera", "Bread for Saturday's meetup"));

  expect(items(menu)).toEqual(["Copy link", "Share", "Bookmark", "Mute thread", "Delete and redraft…", "Delete…"]);
});

test("the More menu of another's post: its link, sharing, the bookmark and the thread's mute, then muting and blocking its author", async () => {
  // An author of this server: no page of another server to open, and no server of its own to mute.
  const app = await renderApp("/", { viewer: "sam" });
  const menu = await openMore(app, postOf(await feedOf("Home timeline"), "Yuki 雪", "週末の散歩、三枚"));

  expect(items(menu)).toEqual(["Copy link", "Share", "Bookmark", "Mute thread", "Mute", "Block…"]);
});

test("the More menu of a visitor: the link and sharing, and the bookmark, which leads to the sign-in", async () => {
  const app = await renderApp("/local", { visitor: true });
  const menu = await openMore(app, postOf(await feedOf("Local timeline"), "Sam Rivera", "Bread for Saturday's meetup"));

  expect(items(menu)).toEqual(["Copy link", "Share", "Bookmark"]);
  await app.user.click(within(menu).getByRole("menuitem", { name: "Bookmark" }));
  expect(await within(screen.getByRole("main")).findByRole("heading", { level: 1, name: "Sign in" })).toBeDefined();
  expect(app.path()).toStartWith("/login");
});

test("deleting the post open on its own page leaves the page without it", async () => {
  const app = await renderApp("/", { viewer: "sam" });
  await app.navigate(app.fake.postPath("sam-bread"));
  const main = within(screen.getByRole("main"));
  expect(await main.findByRole("heading", { level: 1, name: "Post" })).toBeDefined();
  const feed = await feedOf("Thread");

  const menu = await openMore(app, postOf(feed, "Sam Rivera", "Day 3 of the sourdough"));
  await app.user.click(within(menu).getByRole("menuitem", { name: "Delete…" }));
  const confirm = await screen.findByRole("alertdialog", { name: "Delete this post?" });
  await app.user.click(within(confirm).getByRole("button", { name: "Delete" }));

  expect(await screen.findByText("Post deleted")).toBeDefined();
  await waitFor(() => {
    if (
      within(screen.getByRole("main"))
        .queryAllByRole("article")
        .some((post) => post.textContent.includes("Day 3"))
    )
      throw new Error("test: the deleted post is still on its page");
  });
  // The reader deleted it: the page does not say it failed to load, nor offer to load it again.
  expect(main.queryByRole("alert")).toBeNull();
  expect(main.queryByRole("button", { name: "Try again" })).toBeNull();
});
