// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import { markStatus } from "./interactions";
import { fakeAppData, json, page, status } from "./test/fake";
import { probe } from "./test/probe";
import { bookmarksQuery, homeTimelineQuery, useBookmarks } from "./timeline";

const bookmarked = (id: string) =>
  status(id, { bookmarked: true, favourites_count: 0, boosts_count: 0, reactions: [] });

/**
 * AppData whose server answers the bookmarks with b2 and b1 (b2's page then b1's), b2 alone once a take-back of b1's
 * was sent, the home timeline with h, and each write with `write()`; the bookmarks and the home timeline are read.
 */
async function reader(write: (request: Request) => Response | Promise<Response> = () => json(200, {})) {
  let taken = false;
  const { data, requests } = fakeAppData((request) => {
    const url = new URL(request.url);
    if (request.method === "DELETE" && url.pathname === "/api/v1/statuses/b1/bookmark") taken = true;
    if (request.method !== "GET") return write(request);
    if (url.pathname === "/api/v1/bookmarks")
      return json(
        200,
        url.searchParams.get("cursor") === "c2"
          ? { items: taken ? [] : [bookmarked("b1")] }
          : { items: [bookmarked("b2")], next_cursor: "c2" },
      );
    return json(200, { items: [status("h", { bookmarked: false })] });
  });
  await data.queryClient.infiniteQuery({ ...bookmarksQuery(data), staleTime: Infinity });
  await new InfiniteQueryObserver(data.queryClient, bookmarksQuery(data)).fetchNextPage();
  await data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });
  return { data, requests };
}

const shown = (data: Parameters<typeof probe>[0]) => probe(data, useBookmarks).statuses.map((s) => s.id);

test("the bookmarks are read from the server, page after page, newest bookmark first", async () => {
  const { data, requests } = await reader();
  expect(shown(data)).toEqual(["b2", "b1"]);
  expect(requests.map((u) => u.pathname + u.search)).toEqual([
    "/api/v1/bookmarks",
    "/api/v1/bookmarks?cursor=c2",
    "/api/v1/timelines/home",
  ]);
});

test("the server empty, no bookmark shows", async () => {
  const { data } = fakeAppData(() => json(200, page([])));
  await data.queryClient.infiniteQuery({ ...bookmarksQuery(data), staleTime: Infinity });
  expect(probe(data, useBookmarks)).toMatchObject({ status: "success", statuses: [] });
});

test("a bookmark taken back leaves the bookmarks the screen shows at once, and stays out once the server answered", async () => {
  const { promise: answered, resolve: answer } = Promise.withResolvers<Response>();
  const { data } = await reader(() => answered);
  // The bookmarks' screen observes them.
  const unobserve = new InfiniteQueryObserver(data.queryClient, bookmarksQuery(data)).subscribe(() => undefined);
  const done = markStatus(data, "b1", { type: "bookmark", on: false });
  expect(shown(data)).toEqual(["b2"]);
  answer(json(200, { ...bookmarked("b1"), bookmarked: false }));
  await done;
  await data.queryClient.getQueryCache().find({ queryKey: bookmarksQuery(data).queryKey })?.promise;
  expect(shown(data)).toEqual(["b2"]);
  unobserve();
});

test("a bookmark taken back that the server refused shows again", async () => {
  const { data } = await reader(() => json(422, { type: "about:blank", title: "Unprocessable", status: 422 }));
  await markStatus(data, "b1", { type: "bookmark", on: false }).catch(() => undefined);
  expect(shown(data)).toEqual(["b2", "b1"]);
});

test("a new bookmark marks the bookmarks to be read again, so that it shows first the next time they open", async () => {
  const { data } = await reader(() => json(200, bookmarked("h")));
  expect(data.queryClient.getQueryState(bookmarksQuery(data).queryKey)?.isInvalidated).toBe(false);
  await markStatus(data, "h", { type: "bookmark", on: true });
  expect(data.queryClient.getQueryState(bookmarksQuery(data).queryKey)?.isInvalidated).toBe(true);
});
