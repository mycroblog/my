// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import type { StreamEvent } from "@mycroblog/mykit";
import type { AppData } from "./app-data";
import { receiveListsGap, receiveListStatuses } from "./arrivals";
import { shadowed } from "./shadow";
import { startStream } from "./stream";
import { fakeAppData, json, page, status } from "./test/fake";
import { fakeBrowser, fakeStreams } from "./test/tabs";
import { homeTimelineQuery, listTimelineQuery } from "./timeline";

/**
 * A server whose list timelines answer `first()` of the list for their first page and ["1"] for the page after "c2",
 * and whose home timeline answers ["h"].
 */
function server(first: (list: string) => unknown = () => page(["3", "2"], "c2")) {
  const fake = fakeAppData((request) => {
    const url = new URL(request.url);
    const list = /^\/api\/v1\/timelines\/list\/([^/]+)$/.exec(url.pathname)?.[1];
    if (list === undefined) return json(200, page(["h"]));
    return json(200, url.searchParams.get("cursor") === "c2" ? page(["1"]) : first(list));
  });
  const data: AppData = { ...fake.data, connection: { server: "https://example.com" } };
  return { data, requests: fake.requests };
}

/** What a loader of the list reads: the cache while it holds the list and it is not marked, else a fetch. */
const load = (data: AppData, list: string) =>
  data.queryClient.infiniteQuery({ ...listTimelineQuery(data, list), staleTime: Infinity });

/** The IDs of each cached page of `list`'s timeline that show (those gone left out). */
const cached = (data: AppData, list: string) =>
  data.queryClient
    .getQueryData(listTimelineQuery(data, list).queryKey)
    ?.pages.map((p) => p.items.filter((s) => shadowed(data, s) !== undefined).map((s) => s.id));

const home = (data: AppData) =>
  data.queryClient.getQueryData(homeTimelineQuery(data).queryKey)?.pages.map((p) => p.items.map((s) => s.id));

/** Loads the first two pages of `list`: ["3", "2"] and ["1"]. */
async function twoPages(data: AppData, list: string) {
  await load(data, list);
  const observer = new InfiniteQueryObserver(data.queryClient, listTimelineQuery(data, list));
  await observer.fetchNextPage();
  return observer;
}

describe("listTimelineQuery", () => {
  test("reads the list's timeline, page after page by next_cursor", async () => {
    const { data, requests } = server();
    await twoPages(data, "a");
    expect(cached(data, "a")).toEqual([["3", "2"], ["1"]]);
    expect(requests.map((u) => u.pathname + u.search)).toEqual([
      "/api/v1/timelines/list/a",
      "/api/v1/timelines/list/a?cursor=c2",
    ]);
  });

  test("keeps each list's timeline apart", () => {
    const { data } = server();
    expect(listTimelineQuery(data, "a").queryKey).not.toEqual(listTimelineQuery(data, "b").queryKey);
  });
});

describe("a list's new posts, while no screen shows it", () => {
  test("go at once to the top of that list's first page, once, and to no other list nor home", async () => {
    const { data } = server();
    await twoPages(data, "a");
    await load(data, "b");
    await data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });
    receiveListStatuses(data, "a", [status("5"), status("4")]);
    receiveListStatuses(data, "a", [status("4")]);
    expect(cached(data, "a")).toEqual([["5", "4", "3", "2"], ["1"]]);
    expect(cached(data, "b")).toEqual([["3", "2"]]);
    expect(home(data)).toEqual([["h"]]);
  });

  test("a post of a list never read is left to the list's first read", () => {
    const { data } = server();
    receiveListStatuses(data, "a", [status("4")]);
    expect(cached(data, "a")).toBeUndefined();
  });
});

describe("after a break in the stream", () => {
  test("every list read keeps its first page alone and is marked to be read again, without a fetch", async () => {
    const { data, requests } = server();
    await twoPages(data, "a");
    await load(data, "b");
    receiveListsGap(data);
    expect(cached(data, "a")).toEqual([["3", "2"]]);
    for (const list of ["a", "b"])
      expect(data.queryClient.getQueryState(listTimelineQuery(data, list).queryKey)?.isInvalidated).toBe(true);
    await Promise.resolve();
    expect(requests).toHaveLength(3);
  });

  test("the list a screen shows reads its first page again in place of every page", async () => {
    let first = page(["3", "2"], "c2");
    const { data, requests } = server(() => first);
    const observer = await twoPages(data, "a");
    const unobserve = observer.subscribe(() => undefined);
    first = page(["9", "8"], "c2");
    receiveListsGap(data);
    await data.queryClient.getQueryCache().find({ queryKey: listTimelineQuery(data, "a").queryKey })?.promise;
    expect(cached(data, "a")).toEqual([["9", "8"]]);
    expect(requests.at(-1)?.pathname).toBe("/api/v1/timelines/list/a");
    expect(requests.at(-1)?.search).toBe("");
    unobserve();
  });
});

describe("by the stream", () => {
  /** Starts the tab's stream and opens it. */
  async function streaming(data: AppData) {
    const { run, streams } = fakeStreams();
    startStream(data, { tabs: fakeBrowser().join(), run });
    await Bun.sleep(0);
    const [stream] = streams;
    if (stream === undefined) throw new Error("test: no stream started");
    stream.open(false);
    return stream;
  }

  test("a list_update goes to its list's timeline, and an update of home to none of the lists", async () => {
    const { data } = server();
    await load(data, "a");
    await load(data, "b");
    const stream = await streaming(data);
    const listUpdate: StreamEvent = { type: "list_update", listId: "a", status: status("4") };
    stream.send(listUpdate);
    stream.send({ type: "update", status: status("5") });
    expect(cached(data, "a")).toEqual([["4", "3", "2"]]);
    expect(cached(data, "b")).toEqual([["3", "2"]]);
  });

  test("after it connects again, the lists read are read again", async () => {
    const { data } = server();
    await load(data, "a");
    const stream = await streaming(data);
    expect(data.queryClient.getQueryState(listTimelineQuery(data, "a").queryKey)?.isInvalidated).toBe(false);
    stream.open(true);
    expect(data.queryClient.getQueryState(listTimelineQuery(data, "a").queryKey)?.isInvalidated).toBe(true);
  });

  test("a post deleted on the stream leaves the list's timeline", async () => {
    const { data } = server();
    await load(data, "a");
    const stream = await streaming(data);
    stream.send({ type: "delete", id: "2" });
    expect(cached(data, "a")).toEqual([["3"]]);
  });
});
