// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
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
import { fakeAppData, json } from "./test/fake";
import { probe } from "./test/probe";

const account = (id: string) => ({ id, acct: id, username: id, display_name: id });

/**
 * A server where `from` asked to follow the reader, newest first: it lists those still waiting, answers the
 * relationships with each asking, and answers an accept or a reject with the relationship it leaves, the request no
 * longer listed; or with `refuse` (a status) instead of answering.
 */
function server(from: string[], refuse?: number) {
  const waiting = [...from];
  const sent: string[] = [];
  const fake = fakeAppData((r) => {
    const url = new URL(r.url);
    const path = url.pathname.replace("/api/v1", "");
    sent.push(`${r.method} ${path}`);
    if (path === "/accounts/relationships")
      return json(200, {
        items: url.searchParams.getAll("id").map((id) => ({ id, requested_by: waiting.includes(id) })),
      });
    const answer = /^\/follow_requests\/([^/]+)\/(accept|reject)$/.exec(path);
    if (answer) {
      const id = answer[1] ?? "";
      if (refuse !== undefined) return json(refuse, { type: "about:blank", title: "Refused", status: refuse });
      waiting.splice(waiting.indexOf(id), 1);
      return json(200, { id, requested_by: false, followed_by: answer[2] === "accept" });
    }
    return json(200, { items: waiting.map(account) });
  });
  return { ...fake, sent };
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
  const { data, sent } = server(["mio", "marco"]);
  await data.queryClient.infiniteQuery(followRequestsQuery(data));
  expect(shown(data)).toEqual(["mio", "marco"]);
  expect(sent.filter((s) => s === "GET /accounts/relationships")).toHaveLength(1);
  const before = sent.length;
  expect(probe(data, () => useRelationship("mio"))?.requested_by).toBe(true);
  expect(sent).toHaveLength(before);
});

for (const [accept, path, followed] of [
  [true, "accept", true],
  [false, "reject", false],
] as const)
  test(`a request ${accept ? "approved" : "declined"} stays in place with its answer, and the count drops by one`, async () => {
    const { data, sent } = server(["mio", "marco"]);
    await read(data);
    await answer(data)("mio", accept);
    expect(sent).toContain(`POST /follow_requests/mio/${path}`);
    // Answered here, the request keeps its row, its relationship telling the answer.
    expect(shown(data)).toEqual(["mio", "marco"]);
    const relationship = probe(data, () => useRelationship("mio"));
    expect(relationship?.requested_by).toBe(false);
    expect(relationship?.followed_by).toBe(followed);
    expect(await countLater(data)).toBe(1);
    expect(probe(data, () => useRelationship("marco"))?.requested_by).toBe(true);
  });

test("an answer the server refused rejects, and leaves the request waiting and the count as it was", async () => {
  const { data } = server(["mio", "marco"], 404);
  await read(data);
  const outcome = await answer(data)("mio", true).then(
    () => "resolved",
    () => "rejected",
  );
  expect(outcome).toBe("rejected");
  expect(probe(data, () => useRelationship("mio"))?.requested_by).toBe(true);
  expect(await countLater(data)).toBe(2);
  expect(shown(data)).toEqual(["mio", "marco"]);
});

test("a new request told by the stream marks the count to be read again", async () => {
  const { data } = server(["mio"]);
  await data.queryClient.query(followRequestCountQuery(data));
  expect(data.queryClient.getQueryState(followRequestCountQuery(data).queryKey)?.isInvalidated).toBe(false);
  receiveFollowRequests(data);
  expect(data.queryClient.getQueryState(followRequestCountQuery(data).queryKey)?.isInvalidated).toBe(true);
});
