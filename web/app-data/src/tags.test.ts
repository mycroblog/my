// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ProblemError } from "@mycroblog/mykit";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import { followTag, tagQuery, type Hashtag } from "./tags";
import { fakeAppData, json, page } from "./test/fake";
import { tagTimelineQuery } from "./timeline";

const hashtag = (following: boolean) => ({ name: "Baking", url: "https://my.example/tags/baking", following });

describe("tagTimelineQuery", () => {
  test("reads the hashtag's timeline, page after page by next_cursor", async () => {
    const { data, requests } = fakeAppData((request) =>
      json(200, new URL(request.url).searchParams.get("cursor") === "c2" ? page(["1"]) : page(["3", "2"], "c2")),
    );
    await data.queryClient.infiniteQuery({ ...tagTimelineQuery(data, "baking"), staleTime: Infinity });
    const more = await new InfiniteQueryObserver(data.queryClient, tagTimelineQuery(data, "baking")).fetchNextPage();
    expect(more.data?.pages.flatMap((p) => p.items.map((s) => s.id))).toEqual(["3", "2", "1"]);
    expect(more.hasNextPage).toBe(false);
    expect(requests.map((u) => u.pathname + u.search)).toEqual([
      "/api/v1/timelines/tag/baking",
      "/api/v1/timelines/tag/baking?cursor=c2",
    ]);
  });

  test("a hashtag in another script is asked for by its name", async () => {
    const { data, requests } = fakeAppData(() => json(200, page([])));
    await data.queryClient.infiniteQuery({ ...tagTimelineQuery(data, "山"), staleTime: Infinity });
    expect(requests.map((u) => decodeURIComponent(u.pathname))).toEqual(["/api/v1/timelines/tag/山"]);
  });
});

describe("followTag", () => {
  test("follows and unfollows on the server, and the hashtag as cached shows the answer", async () => {
    const sent: string[] = [];
    const { data } = fakeAppData((request) => {
      sent.push(`${request.method} ${new URL(request.url).pathname}`);
      if (request.method === "GET") return json(200, hashtag(false));
      return json(200, hashtag(request.method === "POST"));
    });
    await data.queryClient.query(tagQuery(data, "baking"));
    await followTag(data, "baking", true);
    expect(data.queryClient.getQueryData<Hashtag>(tagQuery(data, "baking").queryKey)?.following).toBe(true);
    await followTag(data, "baking", false);
    expect(data.queryClient.getQueryData<Hashtag>(tagQuery(data, "baking").queryKey)?.following).toBe(false);
    expect(sent).toEqual([
      "GET /api/v1/tags/baking",
      "POST /api/v1/tags/baking/follow",
      "DELETE /api/v1/tags/baking/follow",
    ]);
  });

  test("a follow the server refuses rejects with its problem and leaves the hashtag unfollowed", async () => {
    const tooMany = {
      type: "/api/v1/problems/too-many-followed-tags",
      title: "Too many followed hashtags",
      status: 422,
      detail: "You follow 100 hashtags, the most you may.",
    };
    const { data } = fakeAppData((request) =>
      request.method === "GET" ? json(200, hashtag(false)) : json(422, tooMany),
    );
    await data.queryClient.query(tagQuery(data, "baking"));
    const error: unknown = await followTag(data, "baking", true).then(
      () => undefined,
      (e: unknown) => e,
    );
    expect(error).toBeInstanceOf(ProblemError);
    expect(data.queryClient.getQueryData<Hashtag>(tagQuery(data, "baking").queryKey)?.following).toBe(false);
  });
});
