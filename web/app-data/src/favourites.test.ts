// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { favouritesQuery, homeTimelineQuery, useFavourites } from ".";
import { markStatus } from "./interactions";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

/** sam's posts the reader favourited, the first the oldest favourite: one more than a page of the server holds. */
const starred = Array.from({ length: 21 }, (_, i) => `starred-${String(i + 1)}`);

/**
 * AppData on a server where the reader follows sam and favourited sam's posts of `starred`, and not sam's newest post,
 * `fresh`; the two pages of the favourites and the home timeline are read. `ids` are the favourites' IDs, the newest
 * favourite first, of which the second page holds the last, `oldest`.
 */
async function reader() {
  const { fake, data, requests } = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [...starred, "fresh"].map((key) => ({
        key,
        author: "sam",
        text: `Post ${key}.`,
        visibility: "public",
        language: "en",
      })),
      interactions: starred.map((post) => ({ kind: "favourite", by: "yuki", post })),
    },
  });
  await data.queryClient.infiniteQuery({ ...favouritesQuery(data), staleTime: Infinity });
  await new InfiniteQueryObserver(data.queryClient, favouritesQuery(data)).fetchNextPage();
  await data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });
  const ids = starred.toReversed().map(fake.idOf);
  return { fake, data, requests, ids, oldest: ids.at(-1) ?? "", newest: ids[0] ?? "", fresh: fake.idOf("fresh") };
}

const shown = (data: AppData) => probe(data, useFavourites).statuses.map((s) => s.id);

/** The favourites' screen observes them: a write that marks them reads them again. */
const observed = (data: AppData) =>
  new InfiniteQueryObserver(data.queryClient, favouritesQuery(data)).subscribe(() => undefined);

test("the favourites are read from the server, page after page, newest favourite first", async () => {
  const { data, requests, ids } = await reader();
  expect(shown(data)).toEqual(ids);
  expect(requests).toHaveLength(3);
  expect(requests[0]).toBe("GET /favourites");
  expect(requests[1]).toStartWith("GET /favourites?cursor=");
  expect(requests[2]).toBe("GET /timelines/home");
});

test("the server empty, no favourite shows", async () => {
  const { data } = fakeData({ seed: { people: scenario.people } });
  await data.queryClient.infiniteQuery({ ...favouritesQuery(data), staleTime: Infinity });
  expect(probe(data, useFavourites)).toMatchObject({ status: "success", statuses: [] });
});

test("a favourite taken back leaves the favourites the screen shows at once, and stays out once the server answered", async () => {
  const { fake, data, ids, oldest } = await reader();
  const write = fake.script("DELETE /statuses/{id}/favourite", { hold: "response" });
  const unobserve = observed(data);
  const done = markStatus(data, oldest, { type: "favourite", on: false });
  expect(shown(data)).toEqual(ids.slice(0, -1));
  write.release();
  await done;
  await data.queryClient.getQueryCache().find({ queryKey: favouritesQuery(data).queryKey })?.promise;
  expect(shown(data)).toEqual(ids.slice(0, -1));
  unobserve();
});

test("a favourite taken back that the server refused shows again", async () => {
  const { fake, data, ids, oldest } = await reader();
  fake.script("DELETE /statuses/{id}/favourite", { fail: 422 });
  await markStatus(data, oldest, { type: "favourite", on: false }).catch(() => undefined);
  expect(shown(data)).toEqual(ids);
});

// A reader has a favourite or one reaction on a post, never both (FR-116): a reaction trades the favourite away.
test("a favourite traded for an emoji reaction leaves the favourites at once, and stays out once the server answered", async () => {
  const { fake, data, ids, newest } = await reader();
  const write = fake.script("PUT /statuses/{id}/reaction", { hold: "response" });
  const unobserve = observed(data);
  const done = markStatus(data, newest, { type: "reaction", name: "🎉", on: true });
  expect(shown(data)).toEqual(ids.slice(1));
  write.release();
  await done;
  await data.queryClient.getQueryCache().find({ queryKey: favouritesQuery(data).queryKey })?.promise;
  expect(shown(data)).toEqual(ids.slice(1));
  unobserve();
});

test("a new favourite marks the favourites to be read again, so that it shows first the next time they open", async () => {
  const { data, ids, fresh } = await reader();
  expect(data.queryClient.getQueryState(favouritesQuery(data).queryKey)?.isInvalidated).toBe(false);
  await markStatus(data, fresh, { type: "favourite", on: true });
  expect(data.queryClient.getQueryState(favouritesQuery(data).queryKey)?.isInvalidated).toBe(true);
  const again = await data.queryClient.infiniteQuery({ ...favouritesQuery(data), staleTime: Infinity });
  expect(again.pages[0]?.items.map((s) => s.id).slice(0, 2)).toEqual([fresh, ids[0] ?? ""]);
});

test("the favourites a server is given come before the seed's, newest first, leaving out a post the reader may not see", async () => {
  // sam's `kept` the reader favourited in the seed; `later` and `latest` favourited since, the newest first; `hidden`
  // is for sam's followers, whom the reader is not among.
  const { fake, data } = fakeData({
    seed: {
      people: scenario.people,
      posts: [
        { key: "kept", author: "sam", text: "Kept.", visibility: "public", language: "en" },
        { key: "later", author: "sam", text: "Later.", visibility: "public", language: "en" },
        { key: "latest", author: "sam", text: "Latest.", visibility: "public", language: "en" },
        { key: "hidden", author: "sam", text: "Hidden.", visibility: "private", language: "en" },
      ],
      interactions: [{ kind: "favourite", by: "yuki", post: "kept" }],
    },
    favourites: ["latest", "hidden", "later"],
  });
  const read = await data.queryClient.infiniteQuery({ ...favouritesQuery(data), staleTime: Infinity });
  const items = read.pages.flatMap((p) => p.items);
  expect(items.map((s) => s.id)).toEqual(["latest", "later", "kept"].map(fake.idOf));
  expect(items.every((s) => s.favourited)).toBe(true);
});
