// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import type { AppData } from "@my/app-data";
import { scenario } from "@my/e2e/fake";
import { PostActions } from "../post-actions";
import { fakeData, markupOf } from "../test/feature";
import { Bookmarks, loadBookmarks } from "./bookmarks";

const screen = (data: AppData, locale = "en") =>
  markupOf(
    data,
    <PostActions onCompose={() => undefined}>
      <Bookmarks />
    </PostActions>,
    locale,
  );

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
  const html = screen(data);
  expect(html).toMatch(/role="feed" aria-label="Bookmarks"/);
  expect(html).toContain("The first loaf.");
  expect(html.indexOf("The second loaf.")).toBeLessThan(html.indexOf("The first loaf."));
  expect(screen(data, "ja")).toMatch(/role="feed" aria-label="ブックマーク"/);
});

test("with no bookmarks, the column says so, in en and ja", async () => {
  const { data } = fakeData({ seed: { people: scenario.people } });
  await loadBookmarks(data);
  expect(screen(data)).toContain("No bookmarks yet. Posts you bookmark show here.");
  expect(screen(data, "ja")).toContain("まだブックマークはありません。ブックマークした投稿がここに表示されます。");
});
