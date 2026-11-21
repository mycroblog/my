// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { followTag, tagQuery, type AppData } from "@my/app-data";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { cleanup, screen, within } from "@testing-library/react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
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

/** The hashtag's screen on `data`, alone in the document, for a reader of `locale`. */
function shown(data: AppData, locale = "en", signIn?: (returnTo: string) => void) {
  cleanup();
  renderFeature(
    data,
    <PostActions onCompose={() => undefined}>
      <TagTimeline name="baking" onBack={() => undefined} signIn={signIn} />
    </PostActions>,
    locale,
  );
}

const follows = (before: Element, after: Element) =>
  (before.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;

test("the loader reads the hashtag's timeline, and the column shows its posts under the hashtag's name", async () => {
  const { data, requests } = await opened();
  expect(requests).toContain("GET /timelines/tag/baking");

  shown(data);
  const feed = screen.getByRole("feed", { name: "Posts with #Baking" });
  expect(within(feed).getByText(/^Day 3 of/)).not.toBeNull();
});

test("the head reads Back, then the hashtag as the server first wrote it as the heading, then Follow, and no count", async () => {
  const { data } = await opened();
  shown(data);
  const heading = screen.getByRole("heading", { level: 2 });
  const follow = screen.getByRole("button", { name: "Follow" });

  expect(heading.textContent).toBe("#Baking");
  expect(follows(screen.getByRole("button", { name: "Back" }), heading)).toBe(true);
  expect(follows(heading, follow)).toBe(true);
  expect(follows(follow, screen.getByRole("feed"))).toBe(true);
});

test("a hashtag the reader follows shows Following, in the reader's language", async () => {
  const { data } = await opened({ following: true });

  shown(data);
  expect(screen.getByRole("button", { name: "Following" })).not.toBeNull();
  shown(data, "ja");
  expect(screen.getByRole("button", { name: "フォロー中" })).not.toBeNull();
});

test("a hashtag with no posts says so, in en and ja, and shows no feed", async () => {
  // No post wrote it, so the server names it as it was asked.
  const { data } = await opened({ posts: false });

  shown(data);
  expect(screen.getByText("No posts with #baking yet.")).not.toBeNull();
  expect(screen.queryAllByRole("feed")).toHaveLength(0);
  shown(data, "ja");
  expect(screen.getByText("まだ #baking の投稿はありません。")).not.toBeNull();
});

test("a visitor reads the posts, and is offered Follow", async () => {
  const { data } = await opened({ visitor: true });

  shown(data, "en", () => undefined);
  expect(within(screen.getByRole("feed")).getByText(/^Day 3 of/)).not.toBeNull();
  expect(screen.getByRole("button", { name: "Follow" })).not.toBeNull();
});
