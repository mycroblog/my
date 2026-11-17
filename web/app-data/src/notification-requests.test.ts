// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import type { AppData } from "./app-data";
import {
  notificationRequestQuery,
  notificationRequestsQuery,
  requestNotificationsQuery,
  useAnswerRequest,
  useNotificationRequest,
  useNotificationRequests,
  useRequestAnswer,
  useRequestNotifications,
  type RequestAnswer,
} from "./notification-requests";
import { notificationRequestSummaryQuery, notificationsQuery } from "./notifications";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

/**
 * A server whose notification policy held back two notifications of kiri and one of kobo (the fake's `held`), and
 * which lists one notification, of mio's follow; it keeps an accept or a dismissal as the API says (an accepted
 * account's notifications join the list). `kiri`, `kobo` and `sam` are the IDs of their accounts.
 */
function server() {
  const { fake, data, requests } = fakeData({
    seed: { people: scenario.people, follows: [{ follower: "mio", followee: "yuki" }] },
    held: true,
  });
  const [kiri, kobo, sam] = ["kiri@far.example", "kobo@bread.example", "sam"].map(fake.accountId) as [
    string,
    string,
    string,
  ];
  return { fake, data, requests, kiri, kobo, sam };
}

/** Reads what the notifications and the screens of the held-back ones show; the IDs of the notifications listed. */
async function read(data: AppData, kiri: string) {
  await data.queryClient.query(notificationRequestSummaryQuery(data));
  const listed = await data.queryClient.infiniteQuery(notificationsQuery(data, "all"));
  await data.queryClient.infiniteQuery(notificationRequestsQuery(data));
  await data.queryClient.infiniteQuery(requestNotificationsQuery(data, kiri));
  return listed.pages.flatMap((p) => p.items.map((n) => n.id));
}

/** What a screen shown again shows: the cache while it holds the answer and it is not marked, else a read. */
async function later(data: AppData) {
  const summary = await data.queryClient.query({ ...notificationRequestSummaryQuery(data), staleTime: Infinity });
  const list = await data.queryClient.infiniteQuery({ ...notificationsQuery(data, "all"), staleTime: Infinity });
  return { summary, notifications: list.pages.flatMap((p) => p.items.map((n) => n.id)) };
}

const answer = (data: AppData, id: string, said: RequestAnswer) => probe(data, useAnswerRequest)(id, said);

/**
 * The state the screen of one account's held-back notifications reads, after it asked the server for it. The probe
 * renders once, without effects, as a screen that is already shown: one that met the failure, not one that would
 * retry it on mounting (and read as pending until then).
 */
async function one(data: AppData, id: string) {
  await data.queryClient.query(notificationRequestQuery(data, id)).catch(() => undefined);
  const queries = data.queryClient.getDefaultOptions().queries;
  data.queryClient.setDefaultOptions({ queries: { ...queries, retryOnMount: false } });
  return probe(data, () => useNotificationRequest(id));
}

/** The IDs of the notifications of the account `id` that the screen of its request shows. */
const heldOf = (data: AppData, id: string) => probe(data, () => useRequestNotifications(id)).items.map((n) => n.id);

test("the screens list a row per account, and an account's held-back notifications", async () => {
  const { data, kiri, kobo } = server();
  await read(data, kiri);
  const rows = probe(data, useNotificationRequests).items;
  expect(rows.map((r) => [r.id, r.notifications_count])).toEqual([
    [kiri, 2],
    [kobo, 1],
  ]);
  expect(probe(data, () => useRequestNotifications(kiri)).items.map((n) => n.type)).toEqual(["favourite", "boost"]);
});

test("one account's request is pending until it is read, then read", async () => {
  const { data, kiri } = server();
  expect(probe(data, () => useNotificationRequest(kiri)).status).toBe("pending");
  const read = await one(data, kiri);
  expect(read.status).toBe("success");
  expect(read.request?.notifications_count).toBe(2);
});

test("a request the server no longer lists is gone, not failed", async () => {
  const { data, sam } = server();
  const read = await one(data, sam);
  expect(read.status).toBe("gone");
  expect(read.request).toBeUndefined();
});

test("a request the server failed to answer is failed, not gone", async () => {
  const { fake, data, kiri } = server();
  fake.script("GET /notifications/requests/{id}", { fail: 500 });
  const read = await one(data, kiri);
  expect(read.status).toBe("error");
  expect(read.request).toBeUndefined();
});

test("accepting an account's notifications tells the server, keeps the answer, and reads the count and the notifications again", async () => {
  const { data, requests, kiri, kobo } = server();
  const listed = await read(data, kiri);
  const held = heldOf(data, kiri);
  expect([listed.length, held.length]).toEqual([1, 2]);
  expect(probe(data, () => useRequestAnswer(kiri))).toBeUndefined();
  await answer(data, kiri, "accepted");
  expect(requests).toContain(`POST /notifications/requests/${kiri}/accept`);
  expect(probe(data, () => useRequestAnswer(kiri))).toBe("accepted");
  expect(probe(data, () => useRequestAnswer(kobo))).toBeUndefined();
  const { summary, notifications } = await later(data);
  expect(summary).toEqual({ accounts_count: 1, notifications_count: 1 });
  // mio's follow is newer than what the policy held back.
  expect(notifications).toEqual([...listed, ...held]);
});

test("dismissing an account's notifications tells the server, keeps the answer, and reads the count again", async () => {
  const { data, requests, kiri, kobo } = server();
  const listed = await read(data, kiri);
  await answer(data, kobo, "dismissed");
  expect(requests).toContain(`POST /notifications/requests/${kobo}/dismiss`);
  expect(probe(data, () => useRequestAnswer(kobo))).toBe("dismissed");
  const { summary, notifications } = await later(data);
  expect(summary).toEqual({ accounts_count: 1, notifications_count: 2 });
  expect(notifications).toEqual(listed);
});

test("an answer the server refused rejects, and leaves the request unanswered and the count as it was", async () => {
  const { fake, data, kiri } = server();
  fake.script("POST /notifications/requests/{id}/accept", { fail: 422 });
  await read(data, kiri);
  const outcome = await answer(data, kiri, "accepted").then(
    () => "resolved",
    () => "rejected",
  );
  expect(outcome).toBe("rejected");
  expect(probe(data, () => useRequestAnswer(kiri))).toBeUndefined();
  expect((await later(data)).summary).toEqual({ accounts_count: 2, notifications_count: 3 });
});
