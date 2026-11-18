// SPDX-License-Identifier: AGPL-3.0-only

// A post that matches a keyword filter, on the screens that name it (ADR-0157 §6, §10): folded under the filter's name
// where the filter's contexts name the screen and not where they do not, refolded when a filter is saved, and left out
// when the filter hides. What a folded post is and does is the post's (web/app-components/src/post-filter.dom.test.tsx).

import { expect, test } from "bun:test";
import { saveKeywordFilter } from "@my/app-data";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { renderApp, type TestApp } from "./test/app";

type Filter = NonNullable<NonNullable<Parameters<typeof renderApp>[1]>["settings"]>["filters"];
type Context = NonNullable<Filter>[number]["contexts"][number];

/** A filter named Baking on "bread", folding where `contexts` say, as the fake's settings take it. */
const baking = (
  contexts: Context[],
  action: "warn" | "hide" = "warn",
  keyword = "bread",
): NonNullable<Filter>[number] => ({ title: "Baking", action, contexts, keywords: [{ keyword, whole_word: false }] });

const folded = () => within(screen.getByRole("main")).queryAllByRole("article", { name: /^Matches filter/ });
const posts = () => within(screen.getByRole("main")).queryAllByRole("article");

/** Waits for the screen's posts to be in place. */
const drawn = () =>
  waitFor(() => {
    if (posts().length === 0) throw new Error("test: no post yet");
  });

test("the home timeline folds the posts that match, a boost of one too, under the filter's name", async () => {
  await renderApp("/", { settings: { filters: [baking(["home"])] } });
  await drawn();

  // Four rows of the timeline match; the fourth is on its second page.
  const rows = folded();
  expect(rows).toHaveLength(3);
  for (const row of rows) {
    expect(within(row).getByRole("button", { name: "Show", expanded: false })).not.toBeNull();
    expect(within(row).getByText(/^Matches filter/).textContent).toMatch(/“\s*Baking\s*”/);
  }
});

test("a filter that does not name the home timeline folds nothing there", async () => {
  await renderApp("/", { settings: { filters: [baking(["thread", "account", "public", "notifications"])] } });
  await drawn();

  expect(folded()).toHaveLength(0);
});

test("Show opens a folded post of the timeline, and Hide folds it again", async () => {
  const app = await renderApp("/", { settings: { filters: [baking(["home"])] } });
  await drawn();
  const [row] = folded();
  const before = folded().length;

  await app.user.click(within(row as HTMLElement).getByRole("button", { name: "Show" }));

  expect(folded()).toHaveLength(before - 1);
  await app.user.click(within(screen.getByRole("main")).getByRole("button", { name: "Hide", expanded: true }));
  expect(folded()).toHaveLength(before);
});

const bread = (post: HTMLElement) => /bread|sourdough/i.test(post.textContent);

/** The home timeline under a filter on `contexts`, then the screen of a post that matches it, once the post is drawn. */
async function openedPost(contexts: Context[]) {
  cleanup();
  const app = await renderApp("/", { settings: { filters: [baking(contexts)] } });
  await drawn();
  const link = within(screen.getByRole("main"))
    .getAllByRole("article")
    .filter((article) => /bread/i.test(article.textContent))
    .flatMap((article) => within(article).queryAllByRole("link", { name: /ago$|^\w{3} \d/ }))
    .find((time) => /\/status\//.test(time.getAttribute("href") ?? ""));
  const href = link?.getAttribute("href");
  if (href === null || href === undefined) throw new Error("test: no post of the home timeline mentions bread");

  await app.navigate(href);
  await within(screen.getByRole("main")).findByRole("heading", { level: 1, name: "Post" });
  await waitFor(() => {
    if (posts().length === 0) throw new Error("test: the thread is not drawn");
  });
}

test("a post that matches a filter naming the thread is folded when opened", async () => {
  await openedPost(["thread"]);

  await waitFor(() => {
    if (folded().length === 0) throw new Error("test: the opened post is not folded");
  });
});

test("a post that matches a filter that does not name the thread is not folded when opened", async () => {
  await openedPost(["account", "public", "notifications"]);

  expect(posts().some(bread)).toBe(true);
  expect(folded()).toHaveLength(0);
});

test("a profile folds the account's posts that match when the filter names the account, and not otherwise", async () => {
  await renderApp("/@sam", { settings: { filters: [baking(["account"])] } });
  await waitFor(() => {
    if (folded().length === 0) throw new Error("test: nothing folded yet");
  });
  expect(folded().length).toBeGreaterThan(0);

  cleanup();
  await renderApp("/@sam", { settings: { filters: [baking(["home", "thread"])] } });
  await drawn();
  expect(folded()).toHaveLength(0);
});

test("the notifications fold a mention that matches when the filter names them, and not otherwise", async () => {
  await renderApp("/notifications", { settings: { filters: [baking(["notifications"])] } });
  await waitFor(() => {
    if (folded().length === 0) throw new Error("test: nothing folded yet");
  });
  expect(folded().length).toBeGreaterThan(0);

  cleanup();
  await renderApp("/notifications", { settings: { filters: [baking(["home", "thread", "account", "public"])] } });
  await drawn();
  expect(folded()).toHaveLength(0);
});

/** The home timeline of yuki, drawn with no filter at all. */
async function unfiltered(): Promise<TestApp> {
  const app = await renderApp("/", { settings: { filters: [] } });
  await drawn();
  expect(folded()).toHaveLength(0);
  return app;
}

test("saving a filter folds the open home timeline without a reload", async () => {
  const app = await unfiltered();

  await saveKeywordFilter(app.data, {
    title: "Baking",
    action: "warn",
    contexts: ["home"],
    keywords: [{ keyword: "bread", whole_word: false }],
  });

  await waitFor(() => {
    if (folded().length !== 3) throw new Error(`test: ${String(folded().length)} posts are folded`);
  });
});

test("a filter that hides leaves its posts out of the timeline", async () => {
  await renderApp("/", { settings: { filters: [] } });
  await drawn();
  const trail = (post: HTMLElement) => /trail/i.test(post.textContent);
  const before = posts().filter(trail);
  if (before.length === 0) throw new Error("test: no post of the home timeline mentions trail");

  cleanup();
  await renderApp("/", { settings: { filters: [baking(["home", "thread"], "hide", "trail")] } });
  await drawn();

  expect(posts().filter(trail)).toHaveLength(0);
  expect(folded()).toHaveLength(0);
});
