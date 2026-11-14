// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
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
import { fakeAppData, json, page, status } from "./test/fake";
import { homeTimelineQuery } from "./timeline";

/** A server whose home timeline answers `first` for its first page and `second` for the page after "c2". */
function timeline(first: () => unknown, second: unknown = page(["1"])) {
  return fakeAppData((request) => {
    const cursor = new URL(request.url).searchParams.get("cursor");
    return json(200, cursor === "c2" ? second : first());
  });
}

const load = (data: AppData) => data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });

/** The IDs of each cached page of the home timeline. */
const cached = (data: AppData) =>
  data.queryClient.getQueryData(homeTimelineQuery(data).queryKey)?.pages.map((p) => p.items.map((s) => s.id));

/** Loads the first two pages: ["3", "2"] and ["1"]. */
async function twoPages(first: () => unknown = () => page(["3", "2"], "c2")) {
  const fake = timeline(first);
  await load(fake.data);
  const observer = new InfiniteQueryObserver(fake.data.queryClient, homeTimelineQuery(fake.data));
  await observer.fetchNextPage();
  return { ...fake, observer };
}

describe("while no screen shows the home timeline", () => {
  test("a new status goes at once to the top of the cached first page, once", async () => {
    const { data } = await twoPages();
    receiveHomeStatuses(data, [status("5"), status("4")]);
    receiveHomeStatuses(data, [status("4")]);
    expect(cached(data)).toEqual([["5", "4", "3", "2"], ["1"]]);
  });

  test("a break keeps the first page alone and marks it, so that the next screen reads it again", async () => {
    const { data, requests } = await twoPages();
    receiveHomeGap(data);
    expect(cached(data)).toEqual([["3", "2"]]);
    expect(data.queryClient.getQueryState(homeTimelineQuery(data).queryKey)?.isInvalidated).toBe(true);
    // Nothing shows it, so nothing is fetched until a loader asks.
    await Promise.resolve();
    expect(requests).toHaveLength(2);
  });

  test("a status that arrives while the next page loads goes in once it has", async () => {
    let answer: (r: Response) => void = () => undefined;
    const { data } = fakeAppData((request) =>
      new URL(request.url).searchParams.get("cursor") === "c2"
        ? new Promise<Response>((resolve) => {
            answer = resolve;
          })
        : json(200, page(["3", "2"], "c2")),
    );
    await load(data);
    const more = new InfiniteQueryObserver(data.queryClient, homeTimelineQuery(data)).fetchNextPage();
    await Promise.resolve();
    receiveHomeStatuses(data, [status("4")]);
    answer(json(200, page(["1"])));
    await more;
    await Promise.resolve();
    expect(cached(data)).toEqual([["4", "3", "2"], ["1"]]);
  });

  test("a status before the first page is fetched is left to that fetch", () => {
    const { data } = timeline(() => page(["1"]));
    receiveHomeStatuses(data, [status("2")]);
    expect(cached(data)).toBeUndefined();
  });
});

describe("while a screen shows it", () => {
  test("the policy decides at each arrival; what it takes goes in, what it does not is held", async () => {
    const { data } = await twoPages();
    const asked: Arrival[] = [];
    let takes = false;
    const leave = showHomeTimeline(data, (arrival) => {
      asked.push(arrival);
      return takes;
    });
    receiveHomeStatuses(data, [status("4")]);
    receiveHomeStatuses(data, [status("5")]);
    expect(cached(data)).toEqual([["3", "2"], ["1"]]);
    // The second arrival offers both held statuses.
    expect(asked).toEqual([
      { kind: "statuses", count: 1 },
      { kind: "statuses", count: 2 },
    ]);
    takes = true;
    receiveHomeStatuses(data, [status("6")]);
    expect(cached(data)).toEqual([["6", "5", "4", "3", "2"], ["1"]]);
    leave();
  });

  test("what is still held when the screen goes is put in", async () => {
    const { data } = await twoPages();
    const leave = showHomeTimeline(data, () => false);
    receiveHomeStatuses(data, [status("4")]);
    leave();
    expect(cached(data)).toEqual([["4", "3", "2"], ["1"]]);
    // From then on nothing is held.
    receiveHomeStatuses(data, [status("5")]);
    expect(cached(data)).toEqual([["5", "4", "3", "2"], ["1"]]);
  });

  test("past the most it holds, it is a break: the first page is read again in place of every page", async () => {
    let first = page(["3", "2"], "c2");
    const { data, requests, observer } = await twoPages(() => first);
    // The screen's column observes the timeline.
    const unobserve = observer.subscribe(() => undefined);
    const asked: Arrival[] = [];
    let takes = false;
    const leave = showHomeTimeline(data, (arrival) => {
      asked.push(arrival);
      return takes;
    });
    receiveHomeStatuses(
      data,
      Array.from({ length: maxHeld + 1 }, (_, i) => status(`n${maxHeld - i}`)),
    );
    expect(asked.at(-1)).toEqual({ kind: "gap" });
    first = page(["n200", "n199"], "c2");
    takes = true;
    receiveHomeGap(data);
    await data.queryClient.getQueryCache().find({ queryKey: homeTimelineQuery(data).queryKey })?.promise;
    expect(cached(data)).toEqual([["n200", "n199"]]);
    expect(requests).toHaveLength(3);
    leave();
    unobserve();
  });

  test("a deleted status leaves the timeline and what it holds", async () => {
    const { data } = await twoPages();
    const leave = showHomeTimeline(data, () => false);
    receiveHomeStatuses(data, [status("4")]);
    receiveHomeStatuses(data, [status("5", { boost: status("2") })]);
    receiveDeletion(data, "2");
    receiveDeletion(data, "4");
    expect(cached(data)).toEqual([["3"], ["1"]]);
    leave();
    expect(cached(data)).toEqual([["3"], ["1"]]);
  });

  test("a break the policy holds is read again when the screen goes", async () => {
    const { data } = await twoPages();
    const leave = showHomeTimeline(data, () => false);
    receiveHomeGap(data);
    expect(cached(data)).toEqual([["3", "2"], ["1"]]);
    leave();
    expect(cached(data)).toEqual([["3", "2"]]);
  });
});
