// SPDX-License-Identifier: AGPL-3.0-only

// Which mark an action asks for, from the post as the cache holds it. What the actions do once pressed is in
// post-actions.dom.test.tsx.

import { expect, test } from "bun:test";
import { homeTimelineQuery, type Status } from "@my/app-data";
import { scenario } from "@my/e2e/fake";
import { markOf } from "./post-actions";
import { fakeData } from "./test/feature";

/**
 * A server where the reader follows sam: the reader's own post `own`, on which they put a 🎉; sam's `marked`, which
 * the reader favourited, boosted and bookmarked; and sam's `asks`, a poll of several choices. The home timeline is
 * read: `post(key)` is a post of it as the cache holds it, and `id(key)` its ID.
 */
async function server() {
  const { fake, data, requests } = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [
        { key: "own", author: "yuki", text: "Post 1", visibility: "public", language: "en" },
        { key: "marked", author: "sam", text: "The first loaf.", visibility: "public", language: "en" },
        {
          key: "asks",
          author: "sam",
          text: "Which loaves?",
          visibility: "public",
          language: "en",
          poll: { options: ["Sourdough", "Rye", "Focaccia"], multiple: true, days: 1 },
        },
      ],
      interactions: [
        { kind: "reaction", by: "yuki", post: "own", emoji: "🎉" },
        { kind: "favourite", by: "yuki", post: "marked" },
        { kind: "boost", by: "yuki", post: "marked" },
        { kind: "bookmark", by: "yuki", post: "marked" },
      ],
    },
  });
  const key = homeTimelineQuery(data).queryKey;
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  requests.length = 0;
  const post = (of: string): Status | undefined =>
    data.queryClient.getQueryData(key)?.pages[0]?.items.find((s) => s.id === fake.idOf(of));
  return { fake, data, requests, post, id: fake.idOf };
}

test("each action asks for the mark the post does not have, or takes back the one it has", async () => {
  const { post, id } = await server();
  const [plain, marked] = [post("own"), post("marked")];
  expect(plain?.reactions).toEqual([{ name: "🎉", count: 1, me: true }]);
  expect(marked).toMatchObject({ favourited: true, boosted: true, bookmarked: true });
  expect(markOf({ type: "favourite", id: id("own") }, plain)).toEqual({ type: "favourite", on: true });
  expect(markOf({ type: "favourite", id: id("marked") }, marked)).toEqual({ type: "favourite", on: false });
  expect(markOf({ type: "boost", id: id("marked") }, marked)).toEqual({ type: "boost", on: false });
  expect(markOf({ type: "bookmark", id: id("own") }, plain)).toEqual({ type: "bookmark", on: true });
  expect(markOf({ type: "reaction", id: id("own"), name: "🎉" }, plain)).toEqual({
    type: "reaction",
    name: "🎉",
    on: false,
  });
  expect(markOf({ type: "reaction", id: id("own"), name: "👍" }, plain)).toEqual({
    type: "reaction",
    name: "👍",
    on: true,
  });
  expect(markOf({ type: "reply", id: id("own") }, plain)).toBeUndefined();
  expect(markOf({ type: "favourite", id: id("own") }, undefined)).toBeUndefined();
});
