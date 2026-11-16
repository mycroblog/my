// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import type { AppData } from "./app-data";
import {
  notificationMarkerQuery,
  notificationsQuery,
  receiveNotification,
  useDismissNotification,
  useNotificationReads,
  useUnreadNotifications,
  type NotificationItem,
  type NotificationReads,
} from "./notifications";
import { AppDataProvider } from "./provider";
import { fakeAppData, json } from "./test/fake";
import { probe } from "./test/probe";

/** An ID as the server's: a later one sorts after an earlier one. */
const id = (n: number) => `0190a5d0-0000-7000-8000-${n.toString().padStart(12, "0")}`;

const notification = (n: number, type: NotificationItem["type"] = "favourite") =>
  ({ id: id(n), type, created_at: "2026-11-14T00:00:00Z" }) as NotificationItem;

/**
 * A server whose notifications are those of `ids`, newest first, a page of `size` at a time, with the marker at
 * `marker`; it moves the marker forward as the API says, and keeps each move it was asked.
 */
async function server(ids: number[], marker: number | undefined, size = ids.length) {
  let last = marker === undefined ? undefined : id(marker);
  const moves: string[] = [];
  const answer = () => ({
    ...(last === undefined ? {} : { last_read_id: last }),
    unread_count: ids.filter((n) => last === undefined || id(n) > last).length,
  });
  const fake = fakeAppData(async (request) => {
    const url = new URL(request.url);
    if (url.pathname.endsWith("/notifications/marker")) {
      if (request.method === "PUT") {
        const { last_read_id: to } = (await request.json()) as { last_read_id: string };
        moves.push(to);
        if (last === undefined || to > last) last = to;
      }
      return json(200, answer());
    }
    const cursor = url.searchParams.get("cursor");
    const from = cursor === null ? 0 : ids.findIndex((n) => id(n) === cursor) + 1;
    const items = ids.slice(from, from + size).map((n) => notification(n));
    const next = from + size < ids.length ? { next_cursor: items.at(-1)?.id } : {};
    return json(200, { items, ...next });
  });
  const { data } = fake;
  await data.queryClient.infiniteQuery(notificationsQuery(data, "all"));
  await data.queryClient.query(notificationMarkerQuery(data));
  return { data, moves };
}

/** What the list and the shell read now. */
function read(data: AppData): { reads: NotificationReads; count: number | undefined } {
  let reads: NotificationReads | undefined;
  let count: number | undefined;
  function Probe() {
    reads = useNotificationReads();
    count = useUnreadNotifications();
    return null;
  }
  renderToStaticMarkup(
    <AppDataProvider data={data}>
      <Probe />
    </AppDataProvider>,
  );
  if (reads === undefined) throw new Error("the probe did not render");
  return { reads, count };
}

/** Waits for the marker's moves the reads started. */
const settle = () => Bun.sleep(5);

test("a notification newer than the marker is unread, and the count is the server's", async () => {
  const { data } = await server([5, 4, 3, 2, 1], 2);
  const { reads, count } = read(data);
  expect([5, 4, 3, 2, 1].map((n) => reads.unread(id(n)))).toEqual([true, true, true, false, false]);
  expect(count).toBe(3);
});

test("reading the newest first leaves the marker until the oldest unread one is read, then moves it to the top", async () => {
  const { data, moves } = await server([5, 4, 3, 2, 1], 2);
  read(data).reads.read(id(5));
  read(data).reads.read(id(4));
  await settle();
  expect(moves).toEqual([]);
  const now = read(data);
  expect(now.reads.unread(id(5))).toBe(false);
  expect(now.reads.unread(id(3))).toBe(true);
  expect(now.count).toBe(1);
  now.reads.read(id(3));
  await settle();
  expect(moves).toEqual([id(5)]);
  expect(read(data).count).toBe(0);
});

test("reading the oldest unread one alone moves the marker to it", async () => {
  const { data, moves } = await server([5, 4, 3, 2, 1], 2);
  read(data).reads.read(id(3));
  await settle();
  expect(moves).toEqual([id(3)]);
  expect(read(data).count).toBe(2);
});

test("while the loaded list does not reach the marker, the marker stays", async () => {
  const { data, moves } = await server([5, 4, 3, 2, 1], 1, 2);
  for (const n of [5, 4]) read(data).reads.read(id(n));
  await settle();
  expect(moves).toEqual([]);
  expect(read(data).count).toBe(2);
});

test("a notification at or under the marker is read already, and reading it asks nothing", async () => {
  const { data, moves } = await server([3, 2, 1], 2);
  read(data).reads.read(id(1));
  await settle();
  expect(moves).toEqual([]);
});

test("reading all moves the marker to the newest notification", async () => {
  const { data, moves } = await server([5, 4, 3], undefined);
  await read(data).reads.readAll();
  expect(moves).toEqual([id(5)]);
  expect(read(data).count).toBe(0);
});

test("a notification from the stream goes to the top of the lists it belongs to, unread", async () => {
  const { data } = await server([2, 1], 1);
  await data.queryClient.infiniteQuery(notificationsQuery(data, "mentions"));
  receiveNotification(data, notification(3, "mention"));
  receiveNotification(data, notification(4, "favourite"));
  const ids = (filter: "all" | "mentions") =>
    data.queryClient
      .getQueryData(notificationsQuery(data, filter).queryKey)
      ?.pages.flatMap((p) => p.items.map((n) => n.id));
  expect(ids("all")).toEqual([id(4), id(3), id(2), id(1)]);
  expect(ids("mentions")?.[0]).toBe(id(3));
  expect(ids("mentions")).not.toContain(id(4));
  const { reads, count } = read(data);
  expect(reads.unread(id(4))).toBe(true);
  expect(count).toBe(3);
});

test("the Mentions list asks for the mentions, the quotes and the new posts of subscribed accounts", async () => {
  const { data, requests } = fakeAppData(() => json(200, { items: [] }));
  await data.queryClient.infiniteQuery(notificationsQuery(data, "mentions"));
  expect(requests.map((u) => u.searchParams.getAll("types").toSorted())).toEqual([["mention", "quote", "status"]]);
});

test("a new post of a subscribed account and a quote from the stream go to the Mentions list", async () => {
  const { data } = await server([2, 1], 1);
  await data.queryClient.infiniteQuery(notificationsQuery(data, "mentions"));
  receiveNotification(data, notification(3, "status"));
  receiveNotification(data, notification(4, "quote"));
  receiveNotification(data, notification(5, "follow"));
  const mentions = data.queryClient
    .getQueryData(notificationsQuery(data, "mentions").queryKey)
    ?.pages.flatMap((p) => p.items.map((n) => n.id));
  expect(mentions?.slice(0, 2)).toEqual([id(4), id(3)]);
  expect(mentions).not.toContain(id(5));
});

/**
 * A server with notifications 3, 2 and 1 in both lists, whose answer to a dismissal waits for the test, and the paths
 * of the dismissals it was sent.
 */
async function dismissable() {
  const { promise: answered, resolve: answer } = Promise.withResolvers<Response>();
  const dismissed: string[] = [];
  const { data } = fakeAppData((request) => {
    const url = new URL(request.url);
    if (url.pathname.endsWith("/dismiss")) {
      dismissed.push(`${request.method} ${url.pathname.replace("/api/v1", "")}`);
      return answered;
    }
    if (url.pathname.endsWith("/notifications/marker")) return json(200, { unread_count: 0 });
    const type = url.searchParams.has("types") ? "mention" : "favourite";
    return json(200, { items: [3, 2, 1].map((n) => notification(n, type)) });
  });
  await data.queryClient.infiniteQuery(notificationsQuery(data, "all"));
  await data.queryClient.infiniteQuery(notificationsQuery(data, "mentions"));
  const ids = (filter: "all" | "mentions") =>
    data.queryClient
      .getQueryData(notificationsQuery(data, filter).queryKey)
      ?.pages.flatMap((p) => p.items.map((n) => n.id));
  return { data, dismissed, answer, ids };
}

test("a dismissed notification leaves every list at once, and the server is told", async () => {
  const { data, dismissed, answer, ids } = await dismissable();
  const done = probe(data, useDismissNotification)(id(2));
  expect(ids("all")).toEqual([id(3), id(1)]);
  expect(ids("mentions")).toEqual([id(3), id(1)]);
  answer(new Response(null, { status: 204 }));
  await done;
  expect(dismissed).toEqual([`POST /notifications/${id(2)}/dismiss`]);
  expect(ids("all")).toEqual([id(3), id(1)]);
  expect(ids("mentions")).toEqual([id(3), id(1)]);
});

test("a dismissal the server refuses puts the notification back where it was in every list, and rejects", async () => {
  const { data, answer, ids } = await dismissable();
  const done = probe(
    data,
    useDismissNotification,
  )(id(2)).then(
    () => "resolved",
    () => "rejected",
  );
  expect(ids("all")).toEqual([id(3), id(1)]);
  answer(json(500, { type: "about:blank", title: "Internal Server Error", status: 500 }));
  expect(await done).toBe("rejected");
  expect(ids("all")).toEqual([id(3), id(2), id(1)]);
  expect(ids("mentions")).toEqual([id(3), id(2), id(1)]);
});
