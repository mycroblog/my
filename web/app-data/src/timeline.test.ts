// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { ProblemError } from "@mycroblog/mykit";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { fakeData } from "./test/data";
import { probe, probeOutside } from "./test/probe";
import { homeTimelineQuery, useHomeTimeline } from "./timeline";

/**
 * A server where the reader follows sam, whose `count` posts are their home timeline: 21 make two pages, of 20 and 1.
 * `ids` are the posts' IDs, newest first.
 */
function timeline(count = 21) {
  const keys = Array.from({ length: count }, (_, i) => `loaf-${String(i + 1)}`);
  const made = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: keys.map((key) => ({ key, author: "sam", text: `Post ${key}.`, visibility: "public", language: "en" })),
    },
  });
  return { ...made, ids: keys.toReversed().map(made.fake.idOf) };
}

/** What a loader reads (ADR-0028 §6): the cache when it holds the query and it is not marked, else a fetch. */
const load = (data: AppData) => data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });

describe("homeTimelineQuery", () => {
  test("fetches the first page without a cursor", async () => {
    const { data, requests, ids } = timeline();
    const result = await load(data);
    expect(result.pages.map((p) => p.items.map((s) => s.id))).toEqual([ids.slice(0, 20)]);
    expect(result.pageParams).toEqual([undefined]);
    expect(requests).toEqual(["GET /timelines/home"]);
  });

  test("a loader reads the cache until the query is marked", async () => {
    const { data, requests } = timeline(1);
    await load(data);
    await load(data);
    expect(requests).toHaveLength(1);
    await data.queryClient.invalidateQueries({ queryKey: homeTimelineQuery(data).queryKey });
    await load(data);
    expect(requests).toHaveLength(2);
  });

  test("its key is the one openapi-react-query gives the operation", () => {
    const { data } = timeline(0);
    expect([...homeTimelineQuery(data).queryKey]).toEqual(["get", "/timelines/home"]);
  });

  test("follows next_cursor until the last page", async () => {
    const { data, requests, ids } = timeline();
    await load(data);
    const observer = new InfiniteQueryObserver(data.queryClient, homeTimelineQuery(data));
    expect(observer.getCurrentResult().hasNextPage).toBe(true);
    const second = await observer.fetchNextPage();
    expect(second.data?.pages.flatMap((p) => p.items.map((s) => s.id))).toEqual(ids);
    expect(second.hasNextPage).toBe(false);
    expect(requests).toEqual(["GET /timelines/home", `GET /timelines/home?cursor=${ids[19] ?? ""}`]);
  });

  test("rejects with the problem the server answered, without retrying", async () => {
    const { fake, data, requests } = timeline(1);
    fake.script("GET /timelines/home", { fail: 500 });
    const error: unknown = await load(data).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProblemError);
    expect(requests).toHaveLength(1);
  });
});

/** What a column of the home timeline shows: its state, whether more follows, and the IDs of its statuses. */
const column = (data: AppData) => {
  const { status, hasNextPage, statuses } = probe(data, useHomeTimeline);
  return { status, hasNextPage, ids: statuses.map((s) => s.id) };
};

describe("useHomeTimeline", () => {
  test("shows the statuses of the cached pages, newest first", async () => {
    const { data, ids } = timeline();
    await load(data);
    expect(column(data)).toEqual({ status: "success", hasNextPage: true, ids: ids.slice(0, 20) });
  });

  test("is pending with no statuses before the first page", () => {
    const { data } = timeline();
    expect(column(data)).toEqual({ status: "pending", hasNextPage: false, ids: [] });
  });

  test("after its first page failed, refetch fetches it again", async () => {
    const { fake, data, requests, ids } = timeline(1);
    fake.script("GET /timelines/home", { times: 1, fail: 500 });
    await load(data).catch(() => undefined);
    const { refetch } = probe(data, useHomeTimeline);
    expect(data.queryClient.getQueryState(homeTimelineQuery(data).queryKey)?.status).toBe("error");
    refetch();
    await data.queryClient.getQueryCache().find({ queryKey: homeTimelineQuery(data).queryKey })?.promise;
    expect(requests).toHaveLength(2);
    expect(column(data)).toEqual({ status: "success", hasNextPage: false, ids });
  });

  test("fails outside AppDataProvider", () => {
    expect(() => {
      probeOutside(useHomeTimeline);
    }).toThrow(/outside AppDataProvider/);
  });
});
