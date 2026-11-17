// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import type { PostAction } from "@my/app-components";
import { homeTimelineQuery, shadowed, type Status } from "@my/app-data";
import { scenario } from "@my/e2e/fake";
import { markOf, PostActions, usePostActions, type ComposeRequest } from "./post-actions";
import { fakeData, markupOf } from "./test/feature";

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

/** The handler as a screen under PostActions gets it, with the home timeline of `server()` read. */
async function handler() {
  const made = await server();
  const { data, post } = made;
  const composed: ComposeRequest[] = [];
  let opened = Promise.withResolvers<ComposeRequest>();
  /** The next request to open the composer, once it is made. */
  const nextCompose = () => opened.promise;
  let act: (action: PostAction) => void = () => undefined;
  function Probe() {
    act = usePostActions();
    return null;
  }
  markupOf(
    data,
    <PostActions
      onCompose={(to) => {
        composed.push(to);
        opened.resolve(to);
        opened = Promise.withResolvers<ComposeRequest>();
      }}
    >
      <Probe />
    </PostActions>,
  );
  const shown = (key: string) => {
    const copy = post(key);
    return copy && shadowed(data, copy);
  };
  return { ...made, act, composed, nextCompose, shown };
}

test("Reply and Quote open the composer for the post, and write nothing", async () => {
  const { act, composed, requests, id } = await handler();
  act({ type: "reply", id: id("own") });
  act({ type: "quote", id: id("own") });
  expect(composed).toEqual([
    { kind: "reply", id: id("own") },
    { kind: "quote", id: id("own") },
  ]);
  expect(requests).toEqual([]);
});

test("favourite reaches the server, and the cache shows it", async () => {
  const { fake, act, requests, shown, id } = await handler();
  const sent = fake.script("POST /statuses/{id}/favourite");
  act({ type: "favourite", id: id("asks") });
  expect(shown("asks")?.favourited).toBe(true);
  await sent.arrived();
  expect(requests).toEqual([`POST /statuses/${id("asks")}/favourite`]);
});

test("a vote reaches the server with the poll and every choice, and opens no composer", async () => {
  const { fake, act, composed, post, id } = await handler();
  const sent = fake.script("POST /statuses/{id}/poll/votes");
  const poll = post("asks")?.poll?.id ?? "";
  act({ type: "vote", id: id("asks"), poll, choices: [0, 2] });
  expect((await sent.arrived()).body).toEqual({ poll_id: poll, choices: [0, 2] });
  expect(composed).toEqual([]);
});

test("Edit reads the post's source and opens the composer with it, and writes nothing", async () => {
  const { act, nextCompose, requests, id } = await handler();
  act({ type: "edit", id: id("own") });
  expect(await nextCompose()).toMatchObject({
    kind: "edit",
    id: id("own"),
    source: { id: id("own"), text: "Post 1", visibility: "public" },
  });
  expect(requests).toContain(`GET /statuses/${id("own")}/source`);
  expect(requests.filter((r) => !r.startsWith("GET "))).toEqual([]);
});

test("an action with no mark, or on a post not in the cache, writes nothing", async () => {
  const { act, requests, id } = await handler();
  act({ type: "translate", id: id("own") });
  act({ type: "favourite", id: "00000000-0000-7000-8000-00000000ffff" });
  expect(requests).toEqual([]);
});
