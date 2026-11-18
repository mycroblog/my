// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { ProblemError } from "@mycroblog/mykit";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { followedHashtagsQuery, useFollowedHashtagList } from "./profile";
import { followTag, tagQuery, type Hashtag } from "./tags";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
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

// The settings list what the reader follows (#3202), and an unfollow there leaves the list.
describe("the hashtags the reader follows", () => {
  /** What a screen of the list shows again: the cache while it holds the answer and it is not marked, else a read. */
  async function followed(data: AppData) {
    await data.queryClient.query({ ...followedHashtagsQuery(data), staleTime: Infinity });
    return probe(data, useFollowedHashtagList).items.map((tag) => tag.name);
  }

  test("useFollowedHashtagList is empty and pending until they are read, then lists them latest first, whole", async () => {
    const { data, requests } = fakeData({ followedTags: ["baking", "photography", "hiking"] });
    expect(probe(data, useFollowedHashtagList)).toMatchObject({ items: [], status: "pending" });
    await data.queryClient.query(followedHashtagsQuery(data));
    const list = probe(data, useFollowedHashtagList);
    expect(list.status).toBe("success");
    expect(list.items.map((tag) => tag.name)).toEqual(["hiking", "photography", "baking"]);
    // The server gives the list in one answer: there is no page to ask for.
    expect(list.hasNextPage).toBe(false);
    expect(requests).toEqual(["GET /followed_tags"]);
  });

  test("reports an error when the list could not be read", async () => {
    const { fake, data } = fakeData({ followedTags: ["baking"] });
    fake.script("GET /followed_tags", { fail: 500 });
    await data.queryClient.query(followedHashtagsQuery(data)).catch(() => undefined);
    const queries = data.queryClient.getDefaultOptions().queries;
    data.queryClient.setDefaultOptions({ queries: { ...queries, retryOnMount: false } });
    expect(probe(data, useFollowedHashtagList)).toMatchObject({ items: [], status: "error" });
  });

  test("an unfollow takes the hashtag out of the list, and a follow puts one first, once the write resolves", async () => {
    const { data } = fakeData({ followedTags: ["baking", "photography"] });
    expect(await followed(data)).toEqual(["photography", "baking"]);
    await followTag(data, "baking", false);
    expect(await followed(data)).toEqual(["photography"]);
    await followTag(data, "hiking", true);
    expect(await followed(data)).toEqual(["hiking", "photography"]);
  });

  test("an unfollow the server fails rejects, and the list stays as it was", async () => {
    const { fake, data } = fakeData({ followedTags: ["baking", "photography"] });
    await followed(data);
    fake.script("DELETE /tags/{hashtag}/follow", { fail: 500 });
    const error: unknown = await followTag(data, "baking", false).then(
      () => undefined,
      (e: unknown) => e,
    );
    expect(error).toBeInstanceOf(ProblemError);
    expect(await followed(data)).toEqual(["photography", "baking"]);
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
