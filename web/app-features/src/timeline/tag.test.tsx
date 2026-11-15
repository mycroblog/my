// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { tagQuery, type AppData } from "@my/app-data";
import { PostActions } from "../post-actions";
import { render, server, status } from "../test/server";
import { loadTagTimeline, TagTimeline } from "./tag";

const baking = (following: boolean) => ({ name: "Baking", url: "https://my.example/tags/baking", following });

/** A server with the hashtag `baking`, as first written "Baking", and its posts, read as the screen opens. */
async function opened(posts: unknown[], following = false) {
  const { data, requests } = server({
    "/tags/baking": baking(following),
    "/timelines/tag/baking": { items: posts },
  });
  await loadTagTimeline(data, "baking");
  await data.queryClient.query(tagQuery(data, "baking"));
  return { data, requests };
}

const screen = (data: AppData, locale = "en", signIn?: (returnTo: string) => void) =>
  render(
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
  const { data, requests } = await opened([status("t", "sam")]);
  expect(requests).toContain("GET /timelines/tag/baking");
  const html = screen(data);
  expect(html).toMatch(/role="feed" aria-label="Posts with #Baking"/);
  expect(html).toContain("Post t");
});

test("the head is one row: the hashtag as the server first wrote it, then Follow at its end, and no count", async () => {
  const { data } = await opened([status("t", "sam")]);
  const row = head(screen(data));
  expect(row).toMatch(/^<h2><bdi>#Baking<\/bdi><\/h2><div><button [^>]*><span>Follow<\/span><\/button><\/div>/);
  expect(bare(screen(data))).toMatch(/>Back<\/button><\/p><div><h2>/);
});

test("a hashtag the reader follows shows Following, in the reader's language", async () => {
  const { data } = await opened([], true);
  expect(head(screen(data))).toContain("<span>Following</span>");
  expect(head(screen(data, "ja"))).toContain("<span>フォロー中</span>");
});

test("a hashtag with no posts says so, in en and ja", async () => {
  const { data } = await opened([]);
  expect(screen(data)).toContain("No posts with #Baking yet. Posts with #Baking that reach this server show here.");
  expect(screen(data, "ja")).toContain(
    "まだ #Baking の投稿はありません。このサーバーに届いた #Baking の投稿がここに表示されます。",
  );
  expect(screen(data)).not.toContain('role="feed"');
});

test("a visitor reads the posts, and is offered Follow", async () => {
  const { data } = await opened([status("t", "sam")]);
  const html = screen(data, "en", () => undefined);
  expect(html).toContain("Post t");
  expect(head(html)).toContain("<span>Follow</span>");
});
