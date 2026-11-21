// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import type { AppData } from "@my/app-data";
import { scenario } from "@my/e2e/fake";
import { cleanup, screen, within } from "@testing-library/react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { Bookmarks, loadBookmarks } from "./bookmarks";

/** The bookmarks' screen on `data`, alone in the document, for a reader of `locale`. */
function shown(data: AppData, locale = "en") {
  cleanup();
  renderFeature(
    data,
    <PostActions onCompose={() => undefined}>
      <Bookmarks />
    </PostActions>,
    locale,
  );
}

test("the loader reads the bookmarks, and the column shows them, newest bookmark first", async () => {
  // The reader bookmarked sam's first post, then the second.
  const { data, requests } = fakeData({
    seed: {
      people: scenario.people,
      posts: ["first", "second"].map((key) => ({
        key,
        author: "sam",
        text: `The ${key} loaf.`,
        visibility: "public",
        language: "en",
      })),
      interactions: ["first", "second"].map((post) => ({ kind: "bookmark", by: "yuki", post })),
    },
  });
  await loadBookmarks(data);
  expect(requests).toEqual(["GET /bookmarks"]);

  shown(data);
  const posts = within(screen.getByRole("feed", { name: "Bookmarks" })).getAllByRole("article");
  expect(posts.map((post) => within(post).queryAllByText(/^The (first|second) loaf\.$/)[0]?.textContent)).toEqual([
    "The second loaf.",
    "The first loaf.",
  ]);

  shown(data, "ja");
  expect(screen.getAllByRole("feed", { name: "ブックマーク" })).toHaveLength(1);
});

test("with no bookmarks, the column says so, in en and ja", async () => {
  const { data } = fakeData({ seed: { people: scenario.people } });
  await loadBookmarks(data);

  shown(data);
  expect(screen.getByText("No bookmarks yet.")).not.toBeNull();
  shown(data, "ja");
  expect(screen.getByText("まだブックマークはありません。")).not.toBeNull();
});
