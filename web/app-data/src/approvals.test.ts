// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { interactionRequestsQuery, useAnswerInteraction, useWaitingCount, waitingRequestsQuery } from "./approvals";
import type { InteractionRequest } from "./approvals";
import type { AppData } from "./app-data";
import { shadowed } from "./shadow";
import type { Status } from "./statuses";
import { fakeAppData, json, status } from "./test/fake";
import { probe } from "./test/probe";
import { statusContextQuery, statusQuery, type StatusContext } from "./thread";

/** The reader's post that the requests are of, with one reply under it and one boost. */
const parent = status("p", { replies_count: 1, boosts_count: 1, favourites_count: 0, reactions: [] });
const earlier = status("r0", { in_reply_to_id: "p" });
const reply = status("r1", { in_reply_to_id: "p" });

const request = (id: string, kind: InteractionRequest["kind"], fields: Partial<InteractionRequest> = {}) =>
  ({
    id,
    kind,
    state: "pending",
    created_at: "2026-11-16T00:00:00Z",
    account: { id: "marco" },
    status: parent,
    ...(kind === "announce" ? {} : { interacting_status: reply }),
    ...fields,
  }) as InteractionRequest;

const waiting = [request("q-reply", "reply"), request("q-boost", "announce"), request("q-quote", "quote")];

/**
 * AppData whose server lists `waiting` and answers each request's accept and reject with it answered, with the reader's
 * post and its thread in the cache, and both lists of the requests read.
 */
async function cached(answer: (id: string, accept: boolean) => Response = answered) {
  const sent: string[] = [];
  const { data } = fakeAppData((r) => {
    const path = new URL(r.url).pathname.replace("/api/v1", "");
    sent.push(`${r.method} ${path}`);
    const written = /^\/interaction_requests\/([^/]+)\/(accept|reject)$/.exec(path);
    if (written) return answer(written[1] ?? "", written[2] === "accept");
    return json(200, { items: waiting });
  });
  data.queryClient.setQueryData(statusQuery(data, "p").queryKey, parent);
  data.queryClient.setQueryData(statusContextQuery(data, "p").queryKey, { ancestors: [], descendants: [earlier] });
  await data.queryClient.query(interactionRequestsQuery(data));
  await data.queryClient.infiniteQuery(waitingRequestsQuery(data));
  return { data, sent };
}

function answered(id: string, accept: boolean): Response {
  const found = waiting.find((r) => r.id === id);
  if (found === undefined) return json(404, { type: "about:blank", title: "Not Found", status: 404 });
  return json(200, { ...found, state: accept ? "accepted" : "rejected" });
}

/** The state of the request `id` in the first page's list and in the paged list. */
function states(data: AppData, id: string) {
  const first = data.queryClient.getQueryData<{ items: InteractionRequest[] }>(interactionRequestsQuery(data).queryKey);
  const paged = data.queryClient.getQueryData(waitingRequestsQuery(data).queryKey);
  return {
    first: first?.items.find((r) => r.id === id)?.state,
    paged: paged?.pages.flatMap((p) => p.items).find((r) => r.id === id)?.state,
  };
}

const parentShown = (data: AppData): Status | undefined => {
  const cachedParent = data.queryClient.getQueryData<Status>(statusQuery(data, "p").queryKey);
  return cachedParent && shadowed(data, cachedParent);
};
const thread = (data: AppData) =>
  data.queryClient.getQueryData<StatusContext>(statusContextQuery(data, "p").queryKey)?.descendants.map((s) => s.id);

const answer = (data: AppData) => probe(data, useAnswerInteraction);

test("the count of what waits is undefined until the server answered, then the pending of the first page", async () => {
  const { data } = fakeAppData(() =>
    json(200, { items: [...waiting, request("q-old", "reply", { state: "accepted" })] }),
  );
  expect(probe(data, useWaitingCount)).toBeUndefined();
  await data.queryClient.query(interactionRequestsQuery(data));
  expect(probe(data, useWaitingCount)).toBe(3);
});

test("an approved reply stays listed as approved, joins the cached thread of its post, and counts on the post", async () => {
  const { data, sent } = await cached();
  await answer(data)("q-reply", true);
  expect(sent).toContain("POST /interaction_requests/q-reply/accept");
  expect(states(data, "q-reply")).toEqual({ first: "accepted", paged: "accepted" });
  expect(thread(data)).toEqual(["r0", "r1"]);
  expect(parentShown(data)?.replies_count).toBe(2);
  expect(probe(data, useWaitingCount)).toBe(2);
});

test("an approved boost stays listed as approved and counts on the post at once", async () => {
  const { data } = await cached();
  await answer(data)("q-boost", true);
  expect(states(data, "q-boost")).toEqual({ first: "accepted", paged: "accepted" });
  expect(parentShown(data)?.boosts_count).toBe(2);
  expect(thread(data)).toEqual(["r0"]);
});

test("a declined reply stays listed as declined, and neither joins the thread nor counts", async () => {
  const { data, sent } = await cached();
  await answer(data)("q-reply", false);
  expect(sent).toContain("POST /interaction_requests/q-reply/reject");
  expect(states(data, "q-reply")).toEqual({ first: "rejected", paged: "rejected" });
  expect(thread(data)).toEqual(["r0"]);
  expect(parentShown(data)?.replies_count).toBe(1);
  expect(probe(data, useWaitingCount)).toBe(2);
});

test("a declined boost does not count on the post", async () => {
  const { data } = await cached();
  await answer(data)("q-boost", false);
  expect(states(data, "q-boost")).toEqual({ first: "rejected", paged: "rejected" });
  expect(parentShown(data)?.boosts_count).toBe(1);
});

test("an answer the server refused leaves the request waiting in both lists, and rejects", async () => {
  const { data } = await cached(() =>
    json(404, { type: "about:blank", title: "Not Found", status: 404, detail: "No such request." }),
  );
  const outcome = await answer(data)("q-reply", true).then(
    () => "resolved",
    () => "rejected",
  );
  expect(outcome).toBe("rejected");
  expect(states(data, "q-reply")).toEqual({ first: "pending", paged: "pending" });
  expect(thread(data)).toEqual(["r0"]);
  expect(parentShown(data)?.replies_count).toBe(1);
});
