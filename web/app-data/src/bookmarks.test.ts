// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { markStatus } from "./interactions";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { bookmarksQuery, homeTimelineQuery, useBookmarks } from "./timeline";

/** sam's posts the reader bookmarked, the first the oldest bookmark: one more than a page of the server holds. */
const kept = Array.from({ length: 21 }, (_, i) => `kept-${String(i + 1)}`);

/**
 * AppData on a server where the reader follows sam and bookmarked sam's posts of `kept`, and not sam's newest post,
 * `fresh`; the two pages of the bookmarks and the home timeline are read. `ids` are the bookmarks' IDs, the newest
 * bookmark first, of which the second page holds the last, `oldest`.
 */
async function reader() {
  const { fake, data, requests } = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [...kept, "fresh"].map((key) => ({
        key,
        author: "sam",
        text: `Post ${key}.`,
        visibility: "public",
        language: "en",
      })),
      interactions: kept.map((post) => ({ kind: "bookmark", by: "yuki", post })),
    },
  });
  await data.queryClient.infiniteQuery({ ...bookmarksQuery(data), staleTime: Infinity });
  await new InfiniteQueryObserver(data.queryClient, bookmarksQuery(data)).fetchNextPage();
  await data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });
  const ids = kept.toReversed().map(fake.idOf);
  return { fake, data, requests, ids, oldest: ids.at(-1) ?? "", fresh: fake.idOf("fresh") };
}

const shown = (data: AppData) => probe(data, useBookmarks).statuses.map((s) => s.id);

/** The bookmarks' screen observes them: a write that marks them reads them again. */
const observed = (data: AppData) =>
  new InfiniteQueryObserver(data.queryClient, bookmarksQuery(data)).subscribe(() => undefined);

test("the bookmarks are read from the server, page after page, newest bookmark first", async () => {
  const { data, requests, ids } = await reader();
  expect(shown(data)).toEqual(ids);
  expect(requests).toEqual(["GET /bookmarks", `GET /bookmarks?cursor=${ids[19] ?? ""}`, "GET /timelines/home"]);
});

test("the server empty, no bookmark shows", async () => {
  const { data } = fakeData({ seed: { people: scenario.people } });
  await data.queryClient.infiniteQuery({ ...bookmarksQuery(data), staleTime: Infinity });
  expect(probe(data, useBookmarks)).toMatchObject({ status: "success", statuses: [] });
});

test("a bookmark taken back leaves the bookmarks the screen shows at once, and stays out once the server answered", async () => {
  const { fake, data, ids, oldest } = await reader();
  const write = fake.script("DELETE /statuses/{id}/bookmark", { hold: "response" });
  const unobserve = observed(data);
  const done = markStatus(data, oldest, { type: "bookmark", on: false });
  expect(shown(data)).toEqual(ids.slice(0, -1));
  write.release();
  await done;
  await data.queryClient.getQueryCache().find({ queryKey: bookmarksQuery(data).queryKey })?.promise;
  expect(shown(data)).toEqual(ids.slice(0, -1));
  unobserve();
});

test("a bookmark taken back that the server refused shows again", async () => {
  const { fake, data, ids, oldest } = await reader();
  fake.script("DELETE /statuses/{id}/bookmark", { fail: 422 });
  await markStatus(data, oldest, { type: "bookmark", on: false }).catch(() => undefined);
  expect(shown(data)).toEqual(ids);
});

test("a new bookmark marks the bookmarks to be read again, so that it shows first the next time they open", async () => {
  const { data, ids, fresh } = await reader();
  expect(data.queryClient.getQueryState(bookmarksQuery(data).queryKey)?.isInvalidated).toBe(false);
  await markStatus(data, fresh, { type: "bookmark", on: true });
  expect(data.queryClient.getQueryState(bookmarksQuery(data).queryKey)?.isInvalidated).toBe(true);
  const again = await data.queryClient.infiniteQuery({ ...bookmarksQuery(data), staleTime: Infinity });
  expect(again.pages[0]?.items.map((s) => s.id).slice(0, 2)).toEqual([fresh, ids[0] ?? ""]);
});
