// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { fakeOrigin, scenario } from "@my/e2e/fake";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { receiveListsGap, receiveListStatuses } from "./arrivals";
import { shadowed } from "./shadow";
import { startStream } from "./stream";
import { fakeData } from "./test/data";
import { fakeBrowser, fakeStreams } from "./test/tabs";
import { homeTimelineQuery, listTimelineQuery } from "./timeline";

/** sam's posts, oldest first, one more than a page, and hana's two after them. */
const bread = Array.from({ length: 21 }, (_, i) => `bread-${String(i + 1)}`);
const notes = ["note-1", "note-2"];

/**
 * A server where the reader follows sam and hana and keeps a list of each: the list `a` of sam, whose timeline is two
 * pages (`first`, `second`), and the list `b` of hana, whose timeline is `other`.
 */
function server() {
  const { fake, data, requests } = fakeData({
    seed: {
      people: scenario.people,
      follows: ["sam", "hana"].map((followee) => ({ follower: "yuki", followee })),
      posts: [...bread.map((key) => ["sam", key] as const), ...notes.map((key) => ["hana", key] as const)].map(
        ([author, key]) => ({ key, author, text: `Post ${key}.`, visibility: "public", language: "en" }),
      ),
      lists: [
        { key: "a", owner: "yuki", name: "Bread", members: ["sam"] },
        { key: "b", owner: "yuki", name: "Notes", members: ["hana"] },
      ],
    },
  });
  const [oldest = "", ...later] = bread.map(fake.idOf);
  return {
    fake,
    data: { ...data, connection: { server: fakeOrigin } } satisfies AppData,
    requests,
    a: fake.listPath(0).replace("/lists/", ""),
    b: fake.listPath(1).replace("/lists/", ""),
    first: later.toReversed(),
    second: [oldest],
    other: notes.toReversed().map(fake.idOf),
  };
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

/** Loads the two pages of `list`. */
async function twoPages(data: AppData, list: string) {
  await load(data, list);
  const observer = new InfiniteQueryObserver(data.queryClient, listTimelineQuery(data, list));
  await observer.fetchNextPage();
  return observer;
}

describe("listTimelineQuery", () => {
  test("reads the list's timeline, page after page by next_cursor", async () => {
    const { data, requests, a, first, second } = server();
    await twoPages(data, a);
    expect(cached(data, a)).toEqual([first, second]);
    expect(requests).toEqual([`GET /timelines/list/${a}`, `GET /timelines/list/${a}?cursor=${first.at(-1) ?? ""}`]);
  });

  test("keeps each list's timeline apart", () => {
    const { data, a, b } = server();
    expect(listTimelineQuery(data, a).queryKey).not.toEqual(listTimelineQuery(data, b).queryKey);
  });
});

describe("a list's new posts, while no screen shows it", () => {
  test("go at once to the top of that list's first page, once, and to no other list nor home", async () => {
    const { fake, data, a, b, first, second, other } = server();
    await twoPages(data, a);
    await load(data, b);
    await data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });
    const before = home(data);
    const [five, four] = fake.newPosts(2);
    if (five === undefined || four === undefined) throw new Error("test: the fake made no posts");
    receiveListStatuses(data, a, [five, four]);
    receiveListStatuses(data, a, [four]);
    expect(cached(data, a)).toEqual([[five.id, four.id, ...first], second]);
    expect(cached(data, b)).toEqual([other]);
    expect(home(data)).toEqual(before);
  });

  test("a post of a list never read is left to the list's first read", () => {
    const { fake, data, a } = server();
    receiveListStatuses(data, a, fake.newPosts(1));
    expect(cached(data, a)).toBeUndefined();
  });
});

describe("after a break in the stream", () => {
  test("every list read keeps its first page alone and is marked to be read again, without a fetch", async () => {
    const { data, requests, a, b, first } = server();
    await twoPages(data, a);
    await load(data, b);
    receiveListsGap(data);
    expect(cached(data, a)).toEqual([first]);
    for (const list of [a, b])
      expect(data.queryClient.getQueryState(listTimelineQuery(data, list).queryKey)?.isInvalidated).toBe(true);
    await Promise.resolve();
    expect(requests).toHaveLength(3);
  });

  test("the list a screen shows reads its first page again in place of every page", async () => {
    const { fake, data, requests, a, first } = server();
    const observer = await twoPages(data, a);
    const unobserve = observer.subscribe(() => undefined);
    // sam posted twice while the stream was away.
    const missed = fake.newPosts(2);
    receiveListsGap(data);
    await data.queryClient.getQueryCache().find({ queryKey: listTimelineQuery(data, a).queryKey })?.promise;
    expect(cached(data, a)).toEqual([[...missed.map((s) => s.id), ...first.slice(0, 18)]]);
    expect(requests.at(-1)).toBe(`GET /timelines/list/${a}`);
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
    const { fake, data, a, b, first, other } = server();
    await load(data, a);
    await load(data, b);
    const stream = await streaming(data);
    const [five, four] = fake.newPosts(2);
    if (five === undefined || four === undefined) throw new Error("test: the fake made no posts");
    stream.send({ type: "list_update", listId: a, status: four });
    stream.send({ type: "update", status: five });
    expect(cached(data, a)).toEqual([[four.id, ...first]]);
    expect(cached(data, b)).toEqual([other]);
  });

  test("after it connects again, the lists read are read again", async () => {
    const { data, a } = server();
    await load(data, a);
    const stream = await streaming(data);
    expect(data.queryClient.getQueryState(listTimelineQuery(data, a).queryKey)?.isInvalidated).toBe(false);
    stream.open(true);
    expect(data.queryClient.getQueryState(listTimelineQuery(data, a).queryKey)?.isInvalidated).toBe(true);
  });

  test("a post deleted on the stream leaves the list's timeline", async () => {
    const { data, a, first } = server();
    await load(data, a);
    const stream = await streaming(data);
    stream.send({ type: "delete", id: first[1] ?? "" });
    expect(cached(data, a)).toEqual([first.filter((_, i) => i !== 1)]);
  });
});
