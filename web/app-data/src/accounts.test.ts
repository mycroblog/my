// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import {
  accountQuery,
  accountStatusesQuery,
  changeRelationship,
  relationshipQuery,
  type Relationship,
} from "./accounts";
import { fakeAppData, json, page } from "./test/fake";
import { homeTimelineQuery } from "./timeline";

/** A server that answers a write of a relationship with `following`, and the reads with what they ask. */
function server() {
  return fakeAppData((request) => {
    const path = new URL(request.url).pathname;
    if (path.endsWith("/follow"))
      return json(200, { id: "a", following: request.method === "POST", requested: false, blocking: false });
    if (path.endsWith("/relationships")) return json(200, { items: [{ id: "a", following: false }] });
    if (path.endsWith("/lookup")) return json(200, { id: "a", acct: "alice", followers_count: 1 });
    return json(200, page(["1"]));
  });
}

test("a follow puts the relationship the server answered into the cache", async () => {
  const { data, requests } = server();
  const following = () =>
    data.queryClient.getQueryData<{ items: Relationship[] }>(relationshipQuery(data, "a").queryKey)?.items[0]
      ?.following;
  await data.queryClient.query(relationshipQuery(data, "a"));
  await changeRelationship(data, "a", "follow");
  expect(requests.at(-1)?.pathname).toBe("/api/v1/accounts/a/follow");
  expect(following()).toBe(true);
  await changeRelationship(data, "a", "unfollow");
  expect(following()).toBe(false);
});

test("a change of a relationship marks the account's counts, the home timeline and the account's posts", async () => {
  const { data } = server();
  const { queryClient } = data;
  await queryClient.query(accountQuery(data, "alice"));
  await queryClient.infiniteQuery(homeTimelineQuery(data));
  await queryClient.infiniteQuery(accountStatusesQuery(data, "a", "posts"));
  await queryClient.infiniteQuery(accountStatusesQuery(data, "b", "posts"));
  await changeRelationship(data, "a", "follow");
  const marked = (key: readonly unknown[]) => queryClient.getQueryState(key)?.isInvalidated;
  expect(marked(accountQuery(data, "alice").queryKey)).toBe(true);
  expect(marked(homeTimelineQuery(data).queryKey)).toBe(true);
  expect(marked(accountStatusesQuery(data, "a", "posts").queryKey)).toBe(true);
  expect(marked(accountStatusesQuery(data, "b", "posts").queryKey)).toBe(false);
});

test("the media of an account are its posts without replies, keyed apart from them", () => {
  const { data } = server();
  const posts = accountStatusesQuery(data, "a", "posts").queryKey;
  const media = accountStatusesQuery(data, "a", "media").queryKey;
  expect(posts).not.toEqual(media);
  expect(posts[2]).toEqual(media[2]);
});
