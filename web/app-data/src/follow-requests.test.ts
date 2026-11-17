// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { useRelationship } from "./accounts";
import type { AppData } from "./app-data";
import {
  followRequestCountQuery,
  followRequestsQuery,
  receiveFollowRequests,
  useAnswerFollowRequest,
  useFollowRequestCount,
  useFollowRequests,
} from "./follow-requests";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

/**
 * A server where the people of `from` asked to follow the reader, the first the newest, and nobody follows anybody;
 * `ids` are their accounts' IDs in that order.
 */
function server(from: string[]) {
  const made = fakeData({ seed: { people: scenario.people }, followRequests: { from: from.toReversed() } });
  return { ...made, ids: from.map(made.fake.accountId) };
}

/** Reads the count and the screen's list, as the notifications and the screen do. */
async function read(data: AppData) {
  await data.queryClient.query(followRequestCountQuery(data));
  await data.queryClient.infiniteQuery(followRequestsQuery(data));
}

/**
 * The count the notifications show when the reader comes back to them: the cache while it holds the count and it is
 * not marked, else a read.
 */
async function countLater(data: AppData) {
  await data.queryClient.query({ ...followRequestCountQuery(data), staleTime: Infinity });
  return probe(data, useFollowRequestCount);
}

const answer = (data: AppData) => probe(data, useAnswerFollowRequest);
const shown = (data: AppData) => probe(data, useFollowRequests).items.map((a) => a.id);

test("the count is undefined until the server answered, then the requests of its first page", async () => {
  const { data } = server(["mio", "marco", "sam"]);
  expect(probe(data, useFollowRequestCount)).toBeUndefined();
  await data.queryClient.query(followRequestCountQuery(data));
  expect(probe(data, useFollowRequestCount)).toBe(3);
});

test("the screen reads the requests with the reader's relationships with each account, in one request", async () => {
  const { data, requests, ids } = server(["mio", "marco"]);
  await data.queryClient.infiniteQuery(followRequestsQuery(data));
  expect(shown(data)).toEqual(ids);
  expect(requests.filter((r) => r.startsWith("GET /accounts/relationships"))).toHaveLength(1);
  const before = requests.length;
  expect(probe(data, () => useRelationship(ids[0]))?.requested_by).toBe(true);
  expect(requests).toHaveLength(before);
});

for (const [accept, path, followed] of [
  [true, "accept", true],
  [false, "reject", false],
] as const)
  test(`a request ${accept ? "approved" : "declined"} stays in place with its answer, and the count drops by one`, async () => {
    const { data, requests, ids } = server(["mio", "marco"]);
    const [mio = "", marco = ""] = ids;
    await read(data);
    await answer(data)(mio, accept);
    expect(requests).toContain(`POST /follow_requests/${mio}/${path}`);
    // Answered here, the request keeps its row, its relationship telling the answer.
    expect(shown(data)).toEqual(ids);
    const relationship = probe(data, () => useRelationship(mio));
    expect(relationship?.requested_by).toBe(false);
    expect(relationship?.followed_by).toBe(followed);
    expect(await countLater(data)).toBe(1);
    expect(probe(data, () => useRelationship(marco))?.requested_by).toBe(true);
  });

test("an answer the server refused rejects, and leaves the request waiting and the count as it was", async () => {
  const { fake, data, ids } = server(["mio", "marco"]);
  const [mio = ""] = ids;
  fake.script("POST /follow_requests/{id}/accept", { fail: 404 });
  await read(data);
  const outcome = await answer(data)(mio, true).then(
    () => "resolved",
    () => "rejected",
  );
  expect(outcome).toBe("rejected");
  expect(probe(data, () => useRelationship(mio))?.requested_by).toBe(true);
  expect(await countLater(data)).toBe(2);
  expect(shown(data)).toEqual(ids);
});

test("a new request told by the stream marks the count to be read again", async () => {
  const { data } = server(["mio"]);
  await data.queryClient.query(followRequestCountQuery(data));
  expect(data.queryClient.getQueryState(followRequestCountQuery(data).queryKey)?.isInvalidated).toBe(false);
  receiveFollowRequests(data);
  expect(data.queryClient.getQueryState(followRequestCountQuery(data).queryKey)?.isInvalidated).toBe(true);
});
