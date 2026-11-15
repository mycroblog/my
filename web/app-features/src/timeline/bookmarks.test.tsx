// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import type { AppData } from "@my/app-data";
import { PostActions } from "../post-actions";
import { render, server, status } from "../test/server";
import { Bookmarks, loadBookmarks } from "./bookmarks";

const bookmarked = (id: string) => status(id, "sam", { bookmarked: true });

const screen = (data: AppData, locale = "en") =>
  render(
    data,
    <PostActions onCompose={() => undefined}>
      <Bookmarks />
    </PostActions>,
    locale,
  );

test("the loader reads the bookmarks, and the column shows them, newest bookmark first", async () => {
  const { data, requests } = server({ "/bookmarks": { items: [bookmarked("b2"), bookmarked("b1")] } });
  await loadBookmarks(data);
  expect(requests).toEqual(["GET /bookmarks"]);
  const html = screen(data);
  expect(html).toMatch(/role="feed" aria-label="Bookmarks"/);
  expect(html.indexOf("Post b2")).toBeLessThan(html.indexOf("Post b1"));
  expect(screen(data, "ja")).toMatch(/role="feed" aria-label="ブックマーク"/);
});

test("with no bookmarks, the column says so, in en and ja", async () => {
  const { data } = server({ "/bookmarks": { items: [] } });
  await loadBookmarks(data);
  expect(screen(data)).toContain("No bookmarks yet. Posts you bookmark show here.");
  expect(screen(data, "ja")).toContain("まだブックマークはありません。ブックマークした投稿がここに表示されます。");
});
