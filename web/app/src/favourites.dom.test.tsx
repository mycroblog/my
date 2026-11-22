// SPDX-License-Identifier: AGPL-3.0-only

// The reader's favourites (#3366), what its route decides: the screen at /favourites, headed and titled Favourites,
// with the posts the reader favourited, newest favourite first, the ones the server was given before the seed's; a
// visitor is asked to sign in; a favourite taken back there, or traded for an emoji reaction, leaves the list at once,
// and one made on another screen shows when the list is opened again; the place of Favourites in the rail and the
// Menu, right after Bookmarks, with the glyph of a favourite; and the position the reader scrolled to, kept on the way
// back as on Bookmarks. What the column shows in each state is the feature's test (app-features,
// timeline/favourites.dom.test.tsx), and the order of the places in the tab bar and the sidebar shell/places.test.ts.

import { expect, test } from "bun:test";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { feedOf, postOf } from "./test/posts";
import { onAPhone, rail, tabBar } from "./test/viewport";

const sam = "Sam Rivera";

const rye = "The rye came out dense.";
const starter = "The starter doubled overnight.";
const crumb = "The crumb is finally open.";

/** A server where the reader follows sam, and favourited sam's `older` and then `newer`, and not `other`. */
const server: FakeOptions = {
  seed: {
    people: scenario.people,
    emojis: scenario.emojis,
    follows: [{ follower: "yuki", followee: "sam" }],
    posts: [
      { key: "other", author: "sam", text: rye, visibility: "public", language: "en" },
      { key: "older", author: "sam", text: starter, visibility: "public", language: "en" },
      { key: "newer", author: "sam", text: crumb, visibility: "public", language: "en" },
    ],
    interactions: [
      { kind: "favourite", by: "yuki", post: "older" },
      { kind: "favourite", by: "yuki", post: "newer" },
    ],
  },
};

/** The texts of the posts of `feed`, in its order. */
const listed = (feed: HTMLElement) =>
  within(feed)
    .getAllByRole("article")
    .map((post) => [rye, starter, crumb].find((text) => post.textContent.includes(text)));

/** Waits until the favourites list the posts of `texts`, in their order. */
const lists = (texts: readonly string[]) =>
  waitFor(async () => {
    const now = listed(await feedOf("Favourites"));
    if (JSON.stringify(now) !== JSON.stringify(texts))
      throw new Error(`test: the favourites list ${JSON.stringify(now)}`);
  });

test("the screen at /favourites is headed and titled Favourites, and lists the posts the reader favourited, newest favourite first", async () => {
  const app = await renderApp("/favourites", server);

  expect(await main().findByRole("heading", { level: 1, name: "Favourites" })).toBeDefined();
  expect(app.title()).toContain("Favourites");
  await lists([crumb, starter]);
});

test("a visitor at /favourites is asked to sign in", async () => {
  await renderApp("/favourites", { visitor: true });

  expect(await main().findByText("Sign in to see this screen.")).toBeDefined();
  expect(main().queryAllByRole("feed")).toHaveLength(0);
});

test("the favourites the server was given come first, then the reader's earlier ones", async () => {
  await renderApp("/favourites", { ...server, favourites: ["other"] });

  await lists([rye, crumb, starter]);
});

test("a favourite taken back on the screen leaves the list at once, and stays gone when the list is opened again", async () => {
  const app = await renderApp("/favourites", server);
  const feed = await feedOf("Favourites");
  const write = app.fake.script("DELETE /statuses/{id}/favourite", { hold: "response" });

  await app.user.click(within(postOf(feed, sam, crumb)).getByRole("button", { name: /^Favourite/, pressed: true }));

  await write.arrived();
  await lists([starter]);
  write.release();
  await write.served();

  await app.navigate("/lists");
  await app.navigate("/favourites");
  await lists([starter]);
});

test("a favourite traded for an emoji reaction leaves the list at once, and stays gone once the server answered", async () => {
  const app = await renderApp("/favourites", server);
  const post = postOf(await feedOf("Favourites"), sam, crumb);
  const write = app.fake.script("PUT /statuses/{id}/reaction", { hold: "response" });

  await app.user.click(within(post).getByRole("button", { name: "React" }));
  const grid = await within(await screen.findByRole("dialog", { name: "React" })).findByRole("grid", { name: "Emoji" });
  await app.user.click(
    within(within(grid).getByRole("rowgroup", { name: "Custom emoji" })).getByRole("gridcell", { name: ":teacup:" }),
  );

  await write.arrived();
  await lists([starter]);
  write.release();
  await write.served();
  await lists([starter]);
});

test("a favourite made on another screen shows first when the favourites are opened again", async () => {
  const app = await renderApp("/favourites", server);
  await lists([crumb, starter]);

  await app.navigate("/");
  const write = app.fake.script("POST /statuses/{id}/favourite");
  await app.user.click(
    within(postOf(await feedOf("Home timeline"), sam, rye)).getByRole("button", { name: /^Favourite/, pressed: false }),
  );
  await write.served();

  await app.navigate("/favourites");
  await lists([rye, crumb, starter]);
});

/** The drawing of the glyph of a favourite not made: the Favourite button of sam's post the reader did not favourite. */
const notFavourited = async () =>
  within(postOf(await feedOf("Home timeline"), sam, rye))
    .getByRole("button", { name: /^Favourite/, pressed: false })
    .querySelector("svg")?.innerHTML;

/** The words of `items`, in their order. */
const names = (items: readonly HTMLElement[]) => items.map(words);

test("the rail holds Favourites right after Bookmarks, a link to /favourites with the glyph of a favourite", async () => {
  const app = await renderApp("/", server);
  const glyph = await notFavourited();
  const shown = names(within(rail()).getAllByRole("link"));

  expect(shown.indexOf("Favourites")).toBe(shown.indexOf("Bookmarks") + 1);
  const entry = within(rail()).getByRole("link", { name: "Favourites" });
  expect(entry.getAttribute("href")).toBe("/favourites");
  expect(glyph).toBeDefined();
  expect(entry.querySelector("svg")?.innerHTML).toBe(glyph);

  await app.user.click(entry);
  expect(await main().findByRole("heading", { level: 1, name: "Favourites" })).toBeDefined();
  expect(app.path()).toBe("/favourites");
});

test("on a phone the Menu holds Favourites right after Bookmarks, with the glyph of a favourite, and it opens the favourites", async () => {
  onAPhone();
  const app = await renderApp("/", server);
  const glyph = await notFavourited();

  await app.user.click(within(tabBar()).getByRole("button", { name: /^Menu(?: \d+ unread in direct messages)?$/ }));
  const menu = await screen.findByRole("menu", { name: "Menu" });
  const items = names(within(menu).getAllByRole("menuitem"));
  expect(items.indexOf("Favourites")).toBe(items.indexOf("Bookmarks") + 1);
  expect(within(tabBar()).queryByRole("link", { name: "Favourites" })).toBeNull();
  const entry = within(menu).getByRole("menuitem", { name: "Favourites" });
  expect(glyph).toBeDefined();
  expect(entry.querySelector("svg")?.innerHTML).toBe(glyph);

  await app.user.click(entry);
  expect(await main().findByRole("heading", { level: 1, name: "Favourites" })).toBeDefined();
  expect(app.path()).toBe("/favourites");
  expect(screen.queryByRole("menu")).toBeNull();
});

/**
 * Scrolls the page of `app` to `top`, leaves for /lists, where the page is at its top, and comes back as the
 * browser's Back does; waits until the page is at `top` again.
 */
async function backAt(app: TestApp, top: number) {
  window.scrollTo(0, top);
  window.dispatchEvent(new Event("scroll"));
  await app.navigate("/lists");
  window.scrollTo(0, 0);
  await app.back();
  await waitFor(() => {
    if (window.scrollY !== top) throw new Error(`test: the page is at ${String(window.scrollY)}, not ${String(top)}`);
  });
}

for (const [place, name, options] of [
  ["/bookmarks", "Bookmarks", { ...server, bookmarks: ["older", "newer"] }],
  ["/favourites", "Favourites", server],
] as const)
  test(`the position the reader scrolled ${name} to comes back with it on the way back`, async () => {
    const app = await renderApp(place, options);
    await feedOf(name);
    try {
      await backAt(app, 300);
      expect(app.path()).toBe(place);
    } finally {
      window.scrollTo(0, 0);
    }
  });
