// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { followTag, tagQuery, type AppData } from "@my/app-data";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { PostActions } from "../post-actions";
import { fakeData, markupOf } from "../test/feature";
import { loadTagTimeline, TagTimeline } from "./tag";

/**
 * A server with the hashtag `baking`, which sam's post first wrote "Baking" (or which no post has), read as the
 * screen opens, by a reader who follows it or not, or by a visitor.
 */
async function opened({
  posts = true,
  following = false,
  ...options
}: FakeOptions & { posts?: boolean; following?: boolean } = {}) {
  const { data, requests } = fakeData({
    seed: {
      people: scenario.people,
      posts: posts
        ? [{ key: "t", author: "sam", text: "Day 3 of #Baking.", visibility: "public", language: "en" }]
        : [],
    },
    ...options,
  });
  if (following) await followTag(data, "baking", true);
  await loadTagTimeline(data, "baking");
  await data.queryClient.query(tagQuery(data, "baking"));
  return { data, requests };
}

const screen = (data: AppData, locale = "en", signIn?: (returnTo: string) => void) =>
  markupOf(
    data,
    <PostActions onCompose={() => undefined}>
      <TagTimeline name="baking" onBack={() => undefined} signIn={signIn} />
    </PostActions>,
    locale,
  );

/** The markup without classes and icons, to read the structure. */
const bare = (html: string) => html.replace(/ class="[^"]*"/g, "").replace(/<svg.*?<\/svg>/g, "");

/** The row of the title: the heading and what follows it, up to the column. */
const head = (html: string) => /<h2>.*?<\/div><\/div>/.exec(bare(html))?.[0] ?? "";

test("the loader reads the hashtag's timeline, and the column shows its posts under the hashtag's name", async () => {
  const { data, requests } = await opened();
  expect(requests).toContain("GET /timelines/tag/baking");
  const html = screen(data);
  expect(html).toMatch(/role="feed" aria-label="Posts with #Baking"/);
  expect(html).toContain("Day 3 of ");
});

test("the head is one row: the hashtag as the server first wrote it, then Follow at its end, and no count", async () => {
  const { data } = await opened();
  const row = head(screen(data));
  expect(row).toMatch(/^<h2><bdi>#Baking<\/bdi><\/h2><div><button [^>]*><span>Follow<\/span><\/button><\/div>/);
  expect(bare(screen(data))).toMatch(/>Back<\/button><\/p><div><h2>/);
});

test("a hashtag the reader follows shows Following, in the reader's language", async () => {
  const { data } = await opened({ following: true });
  expect(head(screen(data))).toContain("<span>Following</span>");
  expect(head(screen(data, "ja"))).toContain("<span>フォロー中</span>");
});

test("a hashtag with no posts says so, in en and ja", async () => {
  // No post wrote it, so the server names it as it was asked.
  const { data } = await opened({ posts: false });
  expect(screen(data)).toContain("No posts with #baking yet. Posts with #baking that reach this server show here.");
  expect(screen(data, "ja")).toContain(
    "まだ #baking の投稿はありません。このサーバーに届いた #baking の投稿がここに表示されます。",
  );
  expect(screen(data)).not.toContain('role="feed"');
});

test("a visitor reads the posts, and is offered Follow", async () => {
  const { data } = await opened({ visitor: true });
  const html = screen(data, "en", () => undefined);
  expect(html).toContain("Day 3 of ");
  expect(head(html)).toContain("<span>Follow</span>");
});
