// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
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
import { notificationRequestSummaryQuery, notificationsQuery, type NotificationItem } from "./notifications";
import { fakeAppData, json } from "./test/fake";
import { probe } from "./test/probe";

const account = (id: string) => ({ id, acct: id, username: id, display_name: id });

const notification = (id: string, from: string) =>
  ({ id, type: "favourite", account: account(from), created_at: "2026-11-14T00:00:00Z" }) as NotificationItem;

/**
 * A server whose notification policy held back two notifications of kiri and one of kobo, which keeps an accept or a
 * dismissal as the API says (an accepted account's notifications join the list); `refuse` (a status) answers every
 * answer and every read of one request instead, and the requests it was sent.
 */
function server(refuse?: number) {
  const held = new Map<string, NotificationItem[]>([
    ["kiri", [notification("k2", "kiri"), notification("k1", "kiri")]],
    ["kobo", [notification("b1", "kobo")]],
  ]);
  const listed: NotificationItem[] = [notification("n1", "mio")];
  const sent: string[] = [];
  const request = (id: string) => ({
    id,
    account: account(id),
    notifications_count: held.get(id)?.length ?? 0,
    last_notification: held.get(id)?.[0],
  });
  const fake = fakeAppData((r) => {
    const path = new URL(r.url).pathname.replace("/api/v1", "");
    sent.push(`${r.method} ${path}`);
    if (path === "/notifications") return json(200, { items: listed });
    if (path === "/notifications/requests") return json(200, { items: [...held.keys()].map(request) });
    if (path === "/notifications/requests/summary")
      return json(200, {
        accounts_count: held.size,
        notifications_count: [...held.values()].flat().length,
      });
    const [, id = "", rest] = /^\/notifications\/requests\/([^/]+)(?:\/(.+))?$/.exec(path) ?? [];
    if (refuse !== undefined && rest !== "notifications")
      return json(refuse, { type: "about:blank", title: "Refused", status: refuse });
    const of = held.get(id);
    if (of === undefined) return json(404, { type: "about:blank", title: "Not Found", status: 404 });
    if (rest === "notifications") return json(200, { items: of });
    if (rest === "accept" || rest === "dismiss") {
      if (rest === "accept") listed.unshift(...of);
      held.delete(id);
      return new Response(null, { status: 204 });
    }
    return json(200, request(id));
  });
  return { ...fake, sent };
}

/** Reads what the notifications and the screens of the held-back ones show. */
async function read(data: AppData) {
  await data.queryClient.query(notificationRequestSummaryQuery(data));
  await data.queryClient.infiniteQuery(notificationsQuery(data, "all"));
  await data.queryClient.infiniteQuery(notificationRequestsQuery(data));
  await data.queryClient.infiniteQuery(requestNotificationsQuery(data, "kiri"));
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

test("the screens list a row per account, and an account's held-back notifications", async () => {
  const { data } = server();
  await read(data);
  const rows = probe(data, useNotificationRequests).items;
  expect(rows.map((r) => [r.id, r.notifications_count])).toEqual([
    ["kiri", 2],
    ["kobo", 1],
  ]);
  expect(probe(data, () => useRequestNotifications("kiri")).items.map((n) => n.id)).toEqual(["k2", "k1"]);
});

test("one account's request is pending until it is read, then read", async () => {
  const { data } = server();
  expect(probe(data, () => useNotificationRequest("kiri")).status).toBe("pending");
  const read = await one(data, "kiri");
  expect(read.status).toBe("success");
  expect(read.request?.notifications_count).toBe(2);
});

test("a request the server no longer lists is gone, not failed", async () => {
  const { data } = server();
  const read = await one(data, "sam");
  expect(read.status).toBe("gone");
  expect(read.request).toBeUndefined();
});

test("a request the server failed to answer is failed, not gone", async () => {
  const { data } = server(500);
  const read = await one(data, "kiri");
  expect(read.status).toBe("error");
  expect(read.request).toBeUndefined();
});

test("accepting an account's notifications tells the server, keeps the answer, and reads the count and the notifications again", async () => {
  const { data, sent } = server();
  await read(data);
  expect(probe(data, () => useRequestAnswer("kiri"))).toBeUndefined();
  await answer(data, "kiri", "accepted");
  expect(sent).toContain("POST /notifications/requests/kiri/accept");
  expect(probe(data, () => useRequestAnswer("kiri"))).toBe("accepted");
  expect(probe(data, () => useRequestAnswer("kobo"))).toBeUndefined();
  const { summary, notifications } = await later(data);
  expect(summary).toEqual({ accounts_count: 1, notifications_count: 1 });
  expect(notifications).toEqual(["k2", "k1", "n1"]);
});

test("dismissing an account's notifications tells the server, keeps the answer, and reads the count again", async () => {
  const { data, sent } = server();
  await read(data);
  await answer(data, "kobo", "dismissed");
  expect(sent).toContain("POST /notifications/requests/kobo/dismiss");
  expect(probe(data, () => useRequestAnswer("kobo"))).toBe("dismissed");
  const { summary, notifications } = await later(data);
  expect(summary).toEqual({ accounts_count: 1, notifications_count: 2 });
  expect(notifications).toEqual(["n1"]);
});

test("an answer the server refused rejects, and leaves the request unanswered and the count as it was", async () => {
  const { data } = server(422);
  await read(data);
  const outcome = await answer(data, "kiri", "accepted").then(
    () => "resolved",
    () => "rejected",
  );
  expect(outcome).toBe("rejected");
  expect(probe(data, () => useRequestAnswer("kiri"))).toBeUndefined();
  expect((await later(data)).summary).toEqual({ accounts_count: 2, notifications_count: 3 });
});
