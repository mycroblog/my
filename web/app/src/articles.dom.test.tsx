// SPDX-License-Identifier: AGPL-3.0-only

// A person reads an article (ADR-0160): its card in the home timeline opens its page, which reads its whole text and
// its replies; and a profile lists its articles on a tab of their own.

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp, type TestApp } from "./test/app";
import { feedOf, postOf, postsLike } from "./test/posts";

const title = "A sourdough starter in seven days";
const summary = "What I fed it, when it rose, and the three mistakes that cost me a week. A diary of a jar of flour.";

/** The texts of `elements`, in their order. */
const texts = (elements: HTMLElement[]) => elements.map((element) => element.textContent);

/** The card of the article in `post`: one link named "Article", its title, and its summary. */
const cardOf = (post: HTMLElement) =>
  within(post).getByRole("link", { name: /^Article A sourdough starter in seven days/ });

/** The tabs of the profile in place. */
const tabs = () => within(within(screen.getByRole("main")).getByRole("tablist", { name: "Posts of the account" }));

/** Opens the Articles tab of the profile in place, and returns once it is the one selected. */
async function openArticles(app: TestApp): Promise<void> {
  await app.user.click(tabs().getByRole("tab", { name: "Articles" }));
  await waitFor(() => {
    if (tabs().getByRole("tab", { name: "Articles" }).getAttribute("aria-selected") !== "true")
      throw new Error("test: the Articles tab is not selected");
  });
}

test("an article opens from its card in the home timeline and reads in full, its replies under it", async () => {
  const app = await renderApp("/", { viewer: "marco" });
  const post = postOf(await feedOf("Home timeline"), "Sam Rivera", title);
  const card = cardOf(post);
  expect(card.getAttribute("aria-label") ?? card.textContent.replace(/\s+/g, " ")).toContain(summary);
  expect(within(post).getByRole("img", { name: "A round loaf on a wooden board" })).toBeDefined();

  await app.user.click(card);

  const thread = await feedOf("Thread");
  expect(app.path()).toMatch(/^\/@sam\/status\/[^/]+$/);
  const article = within(postOf(thread, "Sam Rivera", title));
  expect(article.getByRole("heading", { level: 1, name: title })).toBeDefined();
  const paragraphs = texts(article.getAllByRole("paragraph"));
  expect(paragraphs).toContain(summary);
  expect(paragraphs).toContain(
    "Seven days ago I mixed flour and water in a jar. This is what happened next, day by day.",
  );
  expect(paragraphs).toContain("Next week: the first loaf.");
  expect(article.getByRole("img", { name: "A round loaf on a wooden board" })).toBeDefined();
  expect(texts(article.getAllByRole("heading", { level: 2 }))).toEqual(["The first three days", "What I fed it"]);
  expect(texts(article.getAllByRole("heading", { level: 3 }))).toEqual(["The mistakes"]);
  expect(texts(article.getAllByRole("listitem"))).toEqual([
    "Day 1: 50 g flour, 50 g water",
    "Day 2: the same, at the same hour",
    "Day 3: half of it out, then the same",
  ]);
  expect(article.getByRole("blockquote").textContent.trim()).toBe(
    "A starter is ready when it doubles within six hours of a feed.",
  );
  const table = within(article.getByRole("table"));
  expect(texts(table.getAllByRole("columnheader"))).toEqual(["Day", "Flour", "Water"]);
  expect(table.getAllByRole("row")).toHaveLength(4);
  // An image from another host is never fetched by the reader's browser: the server wrote a link of its alt text.
  expect(article.getByRole("link", { name: "the shop on a map" })).toBeDefined();
  expect(article.queryByRole("img", { name: "the shop on a map" })).toBeNull();
  expect(texts(article.getAllByRole("code")).map((code) => code.trimEnd())).toContain(
    "feed: 1 starter : 1 flour : 1 water",
  );
  expect(article.getAllByRole("separator")).toHaveLength(1);

  const reply = postOf(thread, "Yuki 雪", "三日目のにおい、うちでも同じでした");
  expect(reply.textContent).not.toContain("Replying to");
});

test("a profile's Articles tab lists its articles and none of its other posts", async () => {
  const app = await renderApp("/@sam", { viewer: "marco" });
  expect(postOf(await feedOf("Posts of the account"), "Sam Rivera", "Day 3 of the sourdough")).toBeDefined();
  expect(texts(tabs().getAllByRole("tab"))).toEqual(["Posts", "Posts and replies", "Media", "Articles"]);

  await openArticles(app);

  const feed = await feedOf("Posts of the account");
  await waitFor(() => {
    expect(within(feed).getAllByRole("article")).toHaveLength(1);
  });
  await app.user.click(cardOf(postOf(feed, "Sam Rivera", title)));
  const article = postOf(await feedOf("Thread"), "Sam Rivera", title);
  expect(within(article).getByRole("heading", { level: 1, name: title })).toBeDefined();
});

test("a profile's pinned posts do not lead its Articles tab", async () => {
  const app = await renderApp("/@hana", { viewer: "marco" });
  // The pinned post leads the profile's posts, under the line "Pinned", before it comes again in its own place.
  const welcome = postsLike(await feedOf("Posts of the account"), "Hana 花", "mycroblog のデモサーバーへようこそ");
  expect(welcome.filter((post) => post.textContent.startsWith("Pinned"))).toHaveLength(1);

  await openArticles(app);

  const panel = within(await within(screen.getByRole("main")).findByRole("tabpanel", { name: "Articles" }));
  await waitFor(() => {
    if (!texts(panel.queryAllByRole("status")).includes("No posts yet."))
      throw new Error("test: the Articles tab does not say that it holds no posts");
  });
  expect(panel.queryAllByRole("article")).toHaveLength(0);
});
