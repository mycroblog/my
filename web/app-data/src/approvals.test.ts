// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { interactionRequestsQuery, useAnswerInteraction, useWaitingCount, waitingRequestsQuery } from "./approvals";
import type { InteractionRequest } from "./approvals";
import type { AppData } from "./app-data";
import { shadowed } from "./shadow";
import type { Status } from "./statuses";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { statusContextQuery, statusQuery, type StatusContext } from "./thread";

/**
 * AppData on a server where a post of the reader's waits for their answers to marco's reply and boost and mio's quote
 * (the fake's `approvals`), with the post and its thread in the cache, and both lists of the requests read; and the ID
 * of the request of each kind, of the post, and of the reply that waits.
 */
async function cached() {
  const { fake, data, requests } = fakeData({ approvals: true });
  const post = fake.idOf("yuki-asks");
  await data.queryClient.query(statusQuery(data, post));
  await data.queryClient.query(statusContextQuery(data, post));
  const first = await data.queryClient.query(interactionRequestsQuery(data));
  await data.queryClient.infiniteQuery(waitingRequestsQuery(data));
  const of = (kind: InteractionRequest["kind"]) => first.items.find((r) => r.kind === kind)?.id ?? "";
  const ids = { reply: of("reply"), boost: of("announce"), quote: of("quote") };
  return { fake, data, requests, post, ids, reply: fake.idOf("marco-asks-reply") };
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

const parentShown = (data: AppData, post: string): Status | undefined => {
  const cachedParent = data.queryClient.getQueryData<Status>(statusQuery(data, post).queryKey);
  return cachedParent && shadowed(data, cachedParent);
};
const thread = (data: AppData, post: string) =>
  data.queryClient.getQueryData<StatusContext>(statusContextQuery(data, post).queryKey)?.descendants.map((s) => s.id);

const answer = (data: AppData) => probe(data, useAnswerInteraction);

test("the count of what waits is undefined until the server answered, then the pending of the first page", async () => {
  const { data } = fakeData({ approvals: true });
  expect(probe(data, useWaitingCount)).toBeUndefined();
  await data.queryClient.query(interactionRequestsQuery(data));
  expect(probe(data, useWaitingCount)).toBe(3);
});

test("an approved reply stays listed as approved and joins the cached thread of its post", async () => {
  const { data, requests, post, ids, reply } = await cached();
  expect(thread(data, post)).toEqual([]);
  await answer(data)(ids.reply, true);
  expect(requests).toContain(`POST /interaction_requests/${ids.reply}/accept`);
  expect(states(data, ids.reply)).toEqual({ first: "accepted", paged: "accepted" });
  expect(thread(data, post)).toEqual([reply]);
  expect(probe(data, useWaitingCount)).toBe(2);
});

// The answer's post counts the reply already, and the app's data counts it once more: the post shows 2.
test("an approved reply counts once on the post", async () => {
  const { data, post, ids } = await cached();
  expect(parentShown(data, post)?.replies_count).toBe(0);
  await answer(data)(ids.reply, true);
  expect(parentShown(data, post)?.replies_count).toBe(1);
});

test("an approved boost stays listed as approved, and joins no thread", async () => {
  const { data, post, ids } = await cached();
  await answer(data)(ids.boost, true);
  expect(states(data, ids.boost)).toEqual({ first: "accepted", paged: "accepted" });
  expect(thread(data, post)).toEqual([]);
});

// The answer's post counts the boost already, and the app's data counts it once more: the post shows 2.
test("an approved boost counts once on the post, at once", async () => {
  const { data, post, ids } = await cached();
  expect(parentShown(data, post)?.boosts_count).toBe(0);
  await answer(data)(ids.boost, true);
  expect(parentShown(data, post)?.boosts_count).toBe(1);
});

test("a declined reply stays listed as declined, and neither joins the thread nor counts", async () => {
  const { data, requests, post, ids } = await cached();
  await answer(data)(ids.reply, false);
  expect(requests).toContain(`POST /interaction_requests/${ids.reply}/reject`);
  expect(states(data, ids.reply)).toEqual({ first: "rejected", paged: "rejected" });
  expect(thread(data, post)).toEqual([]);
  expect(parentShown(data, post)?.replies_count).toBe(0);
  expect(probe(data, useWaitingCount)).toBe(2);
});

test("a declined boost does not count on the post", async () => {
  const { data, post, ids } = await cached();
  await answer(data)(ids.boost, false);
  expect(states(data, ids.boost)).toEqual({ first: "rejected", paged: "rejected" });
  expect(parentShown(data, post)?.boosts_count).toBe(0);
});

test("an answer the server refused leaves the request waiting in both lists, and rejects", async () => {
  const { fake, data, post, ids } = await cached();
  fake.script("POST /interaction_requests/{id}/accept", { fail: 404 });
  const outcome = await answer(data)(ids.reply, true).then(
    () => "resolved",
    () => "rejected",
  );
  expect(outcome).toBe("rejected");
  expect(states(data, ids.reply)).toEqual({ first: "pending", paged: "pending" });
  expect(thread(data, post)).toEqual([]);
  expect(parentShown(data, post)?.replies_count).toBe(0);
});
