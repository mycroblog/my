// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import {
  maxHeld,
  receiveDeletion,
  receiveHomeGap,
  receiveHomeStatuses,
  showHomeTimeline,
  type Arrival,
} from "./arrivals";
import { shadowed } from "./shadow";
import { fakeData } from "./test/data";
import { homeTimelineQuery } from "./timeline";

/** The posts of the home timeline, oldest first: one more than a page of the server holds. */
const keys = Array.from({ length: 21 }, (_, i) => `post-${String(i + 1)}`);

/** A server where the reader follows sam, whose 21 posts make two pages of the home timeline: 20 and 1. */
function timeline() {
  const made = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: keys.map((key) => ({ key, author: "sam", text: `Loaf ${key}.`, visibility: "public", language: "en" })),
    },
  });
  const [oldest = "", ...later] = keys.map(made.fake.idOf);
  return { ...made, first: later.toReversed(), second: [oldest] };
}

const load = (data: AppData) => data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });

/** The IDs of each cached page of the home timeline, of the statuses that show (those gone left out). */
const cached = (data: AppData) =>
  data.queryClient
    .getQueryData(homeTimelineQuery(data).queryKey)
    ?.pages.map((p) => p.items.filter((s) => shadowed(data, s) !== undefined).map((s) => s.id));

/** Loads the two pages, `first` and `second`. */
async function twoPages() {
  const made = timeline();
  await load(made.data);
  const observer = new InfiniteQueryObserver(made.data.queryClient, homeTimelineQuery(made.data));
  await observer.fetchNextPage();
  return { ...made, observer };
}

describe("while no screen shows the home timeline", () => {
  test("a new status goes at once to the top of the cached first page, once", async () => {
    const { fake, data, first, second } = await twoPages();
    const [five, four] = fake.newPosts(2);
    if (five === undefined || four === undefined) throw new Error("test: the fake made no posts");
    receiveHomeStatuses(data, [five, four]);
    receiveHomeStatuses(data, [four]);
    expect(cached(data)).toEqual([[five.id, four.id, ...first], second]);
  });

  test("a break keeps the first page alone and marks it, so that the next screen reads it again", async () => {
    const { data, requests, first } = await twoPages();
    receiveHomeGap(data);
    expect(cached(data)).toEqual([first]);
    expect(data.queryClient.getQueryState(homeTimelineQuery(data).queryKey)?.isInvalidated).toBe(true);
    // Nothing shows it, so nothing is fetched until a loader asks.
    await Promise.resolve();
    expect(requests).toHaveLength(2);
  });

  test("a status that arrives while the next page loads goes in once it has", async () => {
    const { fake, data, first, second } = timeline();
    await load(data);
    const next = fake.script("GET /timelines/home", { hold: "response" });
    const more = new InfiniteQueryObserver(data.queryClient, homeTimelineQuery(data)).fetchNextPage();
    await next.arrived();
    const arrived = fake.newPosts(1);
    receiveHomeStatuses(data, arrived);
    next.release();
    await more;
    await Promise.resolve();
    expect(cached(data)).toEqual([[...arrived.map((s) => s.id), ...first], second]);
  });

  test("a status before the first page is fetched is left to that fetch", () => {
    const { fake, data } = timeline();
    receiveHomeStatuses(data, fake.newPosts(1));
    expect(cached(data)).toBeUndefined();
  });
});

describe("while a screen shows it", () => {
  test("the policy decides at each arrival; what it takes goes in, what it does not is held", async () => {
    const { fake, data, first, second } = await twoPages();
    const asked: Arrival[] = [];
    let takes = false;
    const leave = showHomeTimeline(data, (arrival) => {
      asked.push(arrival);
      return takes;
    });
    const four = fake.newPosts(1);
    const five = fake.newPosts(1);
    receiveHomeStatuses(data, four);
    receiveHomeStatuses(data, five);
    expect(cached(data)).toEqual([first, second]);
    // The second arrival offers both held statuses.
    expect(asked).toEqual([
      { kind: "statuses", count: 1 },
      { kind: "statuses", count: 2 },
    ]);
    takes = true;
    const six = fake.newPosts(1);
    receiveHomeStatuses(data, six);
    expect(cached(data)).toEqual([[...[...six, ...five, ...four].map((s) => s.id), ...first], second]);
    leave();
  });

  test("what is still held when the screen goes is put in", async () => {
    const { fake, data, first, second } = await twoPages();
    const leave = showHomeTimeline(data, () => false);
    const [four] = fake.newPosts(1);
    const [five] = fake.newPosts(1);
    if (four === undefined || five === undefined) throw new Error("test: the fake made no posts");
    receiveHomeStatuses(data, [four]);
    leave();
    expect(cached(data)).toEqual([[four.id, ...first], second]);
    // From then on nothing is held.
    receiveHomeStatuses(data, [five]);
    expect(cached(data)).toEqual([[five.id, four.id, ...first], second]);
  });

  test("past the most it holds, it is a break: the first page is read again in place of every page", async () => {
    const { fake, data, requests, observer } = await twoPages();
    // The screen's column observes the timeline.
    const unobserve = observer.subscribe(() => undefined);
    const asked: Arrival[] = [];
    let takes = false;
    const leave = showHomeTimeline(data, (arrival) => {
      asked.push(arrival);
      return takes;
    });
    const arrived = fake.newPosts(maxHeld + 1);
    receiveHomeStatuses(data, arrived);
    expect(asked.at(-1)).toEqual({ kind: "gap" });
    takes = true;
    receiveHomeGap(data);
    await data.queryClient.getQueryCache().find({ queryKey: homeTimelineQuery(data).queryKey })?.promise;
    expect(cached(data)).toEqual([arrived.slice(0, 20).map((s) => s.id)]);
    expect(requests).toHaveLength(3);
    leave();
    unobserve();
  });

  test("a deleted status leaves the timeline and what it holds", async () => {
    const { fake, data, first, second } = await twoPages();
    const leave = showHomeTimeline(data, () => false);
    const [four, five] = fake.newPosts(2).toReversed();
    const [newest, ...rest] = data.queryClient.getQueryData(homeTimelineQuery(data).queryKey)?.pages[0]?.items ?? [];
    if (four === undefined || five === undefined || newest === undefined) throw new Error("test: no posts");
    receiveHomeStatuses(data, [four]);
    // A boost of the page's newest post arrives.
    receiveHomeStatuses(data, [{ ...five, boost: newest }]);
    receiveDeletion(data, newest.id);
    receiveDeletion(data, four.id);
    expect(first[0]).toBe(newest.id);
    expect(cached(data)).toEqual([rest.map((s) => s.id), second]);
    leave();
    expect(cached(data)).toEqual([rest.map((s) => s.id), second]);
  });

  test("a break the policy holds is read again when the screen goes", async () => {
    const { data, first, second } = await twoPages();
    const leave = showHomeTimeline(data, () => false);
    receiveHomeGap(data);
    expect(cached(data)).toEqual([first, second]);
    leave();
    expect(cached(data)).toEqual([first]);
  });
});
