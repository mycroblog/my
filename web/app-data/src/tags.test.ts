// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { ProblemError } from "@mycroblog/mykit";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import { followTag, tagQuery, type Hashtag } from "./tags";
import { fakeData } from "./test/data";
import { tagTimelineQuery } from "./timeline";

describe("tagTimelineQuery", () => {
  test("reads the hashtag's timeline, page after page by next_cursor", async () => {
    // One more post with the hashtag than a page of the server holds.
    const keys = Array.from({ length: 21 }, (_, i) => `loaf-${String(i + 1)}`);
    const { fake, data, requests } = fakeData({
      seed: {
        people: scenario.people,
        posts: keys.map((key) => ({
          key,
          author: "sam",
          text: `${key} #baking`,
          visibility: "public",
          language: "en",
        })),
      },
    });
    const ids = keys.toReversed().map(fake.idOf);
    await data.queryClient.infiniteQuery({ ...tagTimelineQuery(data, "baking"), staleTime: Infinity });
    const more = await new InfiniteQueryObserver(data.queryClient, tagTimelineQuery(data, "baking")).fetchNextPage();
    expect(more.data?.pages.map((p) => p.items.map((s) => s.id))).toEqual([ids.slice(0, 20), ids.slice(20)]);
    expect(more.hasNextPage).toBe(false);
    expect(requests).toEqual(["GET /timelines/tag/baking", `GET /timelines/tag/baking?cursor=${ids[19] ?? ""}`]);
  });

  test("a hashtag in another script is asked for by its name", async () => {
    const { fake, data, requests } = fakeData();
    const read = await data.queryClient.infiniteQuery({ ...tagTimelineQuery(data, "山"), staleTime: Infinity });
    expect(requests.map((r) => decodeURIComponent(r))).toEqual(["GET /timelines/tag/山"]);
    expect(read.pages[0]?.items.map((s) => s.id)).toEqual([fake.idOf("yuki-ridge")]);
  });
});

describe("followTag", () => {
  test("follows and unfollows on the server, and the hashtag as cached shows the answer", async () => {
    const { data, requests } = fakeData();
    const following = () => data.queryClient.getQueryData<Hashtag>(tagQuery(data, "baking").queryKey)?.following;
    await data.queryClient.query(tagQuery(data, "baking"));
    expect(following()).toBe(false);
    await followTag(data, "baking", true);
    expect(following()).toBe(true);
    await followTag(data, "baking", false);
    expect(following()).toBe(false);
    expect(requests).toEqual(["GET /tags/baking", "POST /tags/baking/follow", "DELETE /tags/baking/follow"]);
  });

  test("a follow the server refuses rejects with its problem and leaves the hashtag unfollowed", async () => {
    // The reader follows as many hashtags as they may.
    const { data } = fakeData({ followedTagLimit: 0 });
    await data.queryClient.query(tagQuery(data, "baking"));
    const error: unknown = await followTag(data, "baking", true).then(
      () => undefined,
      (e: unknown) => e,
    );
    expect(error).toBeInstanceOf(ProblemError);
    expect((error as ProblemError).problem.type).toBe("/api/v1/problems/too-many-followed-tags");
    expect(data.queryClient.getQueryData<Hashtag>(tagQuery(data, "baking").queryKey)?.following).toBe(false);
  });
});
