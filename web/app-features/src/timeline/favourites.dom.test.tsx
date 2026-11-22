// SPDX-License-Identifier: AGPL-3.0-only

// The reader's favourites as a feature (#3366): what its loader reads, the column it shows under the name Favourites,
// newest favourite first, its states (loading, empty, failed with Try again, the next page read near its end), and a
// favourite taken back there, which leaves the column at once. That a reaction trades a favourite away is the data's
// test (app-data, favourites.test.ts) and the route's; the route, its sign-in prompt and its place in the menu are the
// app's (web/app, favourites.dom.test.tsx).

import { expect, onTestFinished, test } from "bun:test";
import type { AppData } from "@my/app-data";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { Favourites, loadFavourites } from "./favourites";

/** The favourites' screen on `data`, alone in the document, for a reader of `locale`. */
function shown(data: AppData, locale = "en") {
  cleanup();
  return renderFeature(
    data,
    <PostActions onCompose={() => undefined}>
      <Favourites />
    </PostActions>,
    locale,
  );
}

/** A server where the reader favourited sam's first post, then the second. */
const twoFavourited: FakeOptions = {
  seed: {
    people: scenario.people,
    posts: ["first", "second"].map((key) => ({
      key,
      author: "sam",
      text: `The ${key} loaf.`,
      visibility: "public",
      language: "en",
    })),
    interactions: ["first", "second"].map((post) => ({ kind: "favourite", by: "yuki", post })),
  },
};

/** The texts of the posts of the column, in its order. */
const loaves = () =>
  within(screen.getByRole("feed", { name: "Favourites" }))
    .getAllByRole("article")
    .map((post) => within(post).queryAllByText(/^The (first|second) loaf\.$/)[0]?.textContent);

test("the loader reads the favourites, and the column shows them under its name, newest favourite first", async () => {
  const { data, requests } = fakeData(twoFavourited);
  await loadFavourites(data);
  expect(requests).toEqual(["GET /favourites"]);

  shown(data);
  expect(loaves()).toEqual(["The second loaf.", "The first loaf."]);
});

test("with no favourites, the column says so, in en and ja, and shows no feed", async () => {
  const { data } = fakeData({ seed: { people: scenario.people } });
  await loadFavourites(data);

  shown(data);
  expect(screen.getByText("No favourites yet.")).not.toBeNull();
  expect(screen.queryAllByRole("feed")).toHaveLength(0);
  shown(data, "ja");
  expect(screen.getByText("まだお気に入りはありません。")).not.toBeNull();
});

test("while the favourites are on their way, the column says that it loads them", async () => {
  const { fake, data } = fakeData(twoFavourited);
  const read = fake.script("GET /favourites", { hold: "request" });
  const loading = loadFavourites(data);
  shown(data);
  await read.arrived();

  expect(screen.getByText("Loading posts…").getAttribute("role")).toBe("status");
  expect(screen.queryAllByRole("article")).toHaveLength(0);

  read.release();
  await loading;
  await screen.findByRole("feed", { name: "Favourites" });
  expect(screen.queryByText("Loading posts…")).toBeNull();
});

test("favourites that cannot be read say so, and Try again reads them", async () => {
  const { fake, data } = fakeData(twoFavourited);
  const failing = fake.script("GET /favourites", { fail: 500 });
  await loadFavourites(data);
  const user = shown(data);

  const failed = await screen.findByRole("alert");
  expect(failed.textContent).toStartWith("The posts could not be loaded.");
  expect(screen.queryAllByRole("feed")).toHaveLength(0);

  failing.end();
  await user.click(within(failed).getByRole("button", { name: "Try again" }));

  await screen.findByRole("feed", { name: "Favourites" });
  expect(loaves()).toEqual(["The second loaf.", "The first loaf."]);
  expect(screen.queryAllByRole("alert")).toHaveLength(0);
});

/** The browser's IntersectionObserver, held by the test: `reach` says that everything watched came into view. */
function watching() {
  const watchers: { callback: IntersectionObserverCallback; watched: Element[]; self: IntersectionObserver }[] = [];
  const real = globalThis.IntersectionObserver;
  class Held implements IntersectionObserver {
    readonly root = null;
    readonly rootMargin = "";
    readonly scrollMargin = "";
    readonly thresholds = [];
    readonly #watcher: (typeof watchers)[number];
    constructor(callback: IntersectionObserverCallback) {
      this.#watcher = { callback, watched: [], self: this };
      watchers.push(this.#watcher);
    }
    observe(target: Element) {
      this.#watcher.watched.push(target);
    }
    unobserve(target: Element) {
      this.#watcher.watched = this.#watcher.watched.filter((element) => element !== target);
    }
    disconnect() {
      this.#watcher.watched = [];
    }
    takeRecords() {
      return [];
    }
  }
  globalThis.IntersectionObserver = Held;
  onTestFinished(() => {
    globalThis.IntersectionObserver = real;
  });
  return {
    reach: () => {
      act(() => {
        for (const { callback, watched, self } of watchers)
          if (watched.length > 0)
            callback(
              watched.map(
                (target) => ({ isIntersecting: true, intersectionRatio: 1, target }) as IntersectionObserverEntry,
              ),
              self,
            );
      });
    },
  };
}

test("near the end of the column, the next page of favourites is read and shows under the first", async () => {
  // Twenty-one favourites: one more than a page of the server holds.
  const keys = Array.from({ length: 21 }, (_, i) => `loaf-${String(i + 1)}`);
  const { data, requests } = fakeData({
    seed: {
      people: scenario.people,
      posts: keys.map((key) => ({ key, author: "sam", text: `Bake ${key}.`, visibility: "public", language: "en" })),
      interactions: keys.map((post) => ({ kind: "favourite", by: "yuki", post })),
    },
  });
  const { reach } = watching();
  await loadFavourites(data);
  shown(data);
  const posts = () => within(screen.getByRole("feed", { name: "Favourites" })).getAllByRole("article");
  expect(posts()).toHaveLength(20);

  reach();

  await waitFor(() => {
    if (posts().length !== 21) throw new Error(`test: the column shows ${String(posts().length)} posts`);
  });
  // The oldest favourite comes last.
  expect(posts().at(-1)?.textContent).toContain("Bake loaf-1.");
  expect(requests.filter((r) => r.startsWith("GET /favourites?cursor="))).toHaveLength(1);
});

test("a favourite taken back in the column leaves it at once, before the server answered", async () => {
  const { fake, data } = fakeData(twoFavourited);
  await loadFavourites(data);
  const user = shown(data);
  const write = fake.script("DELETE /statuses/{id}/favourite", { hold: "response" });
  const [second] = within(screen.getByRole("feed", { name: "Favourites" })).getAllByRole("article");
  if (second === undefined) throw new Error("test: the column shows no post");

  await user.click(within(second).getByRole("button", { name: /^Favourite/, pressed: true }));

  const { url } = await write.arrived();
  expect(url.pathname).toEndWith(`/statuses/${fake.idOf("second")}/favourite`);
  await waitFor(() => {
    if (loaves().length !== 1) throw new Error(`test: the column shows ${String(loaves().length)} posts`);
  });
  expect(loaves()).toEqual(["The first loaf."]);
  write.release();
  await write.served();
  expect(loaves()).toEqual(["The first loaf."]);
});
