// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import type { AppData } from "./app-data";
import {
  notificationMarkerQuery,
  notificationsQuery,
  receiveNotification,
  useDismissNotification,
  useNotificationReads,
  useUnreadNotifications,
  type NotificationReads,
} from "./notifications";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

/**
 * A server whose notifications are `count` mentions of the reader by sam, of which the reader has not read the
 * `unread` newest; the first page of the list and the marker are read. `id(n)` is the ID of the `n`th notification,
 * 1 the oldest, and `moves()` each place the server was asked to move the marker to.
 */
async function server(count: number, unread: number) {
  const { fake, data, requests } = fakeData({
    seed: {
      people: scenario.people,
      posts: Array.from({ length: count }, (_, i) => ({
        key: `mention-${String(i + 1)}`,
        author: "sam",
        text: `@yuki Loaf ${String(i + 1)} is out of the oven.`,
        visibility: "public" as const,
        language: "en",
      })),
    },
    unreadNotifications: unread,
  });
  const moved = fake.script("PUT /notifications/marker");
  // The server pages them 20 at a time; a test of more reads them all here to name them.
  const all: string[] = [];
  for (let cursor: string | undefined; ;) {
    const { data: page } = await data.api.GET("/notifications", {
      params: { query: cursor === undefined ? {} : { cursor } },
    });
    all.push(...(page?.items.map((n) => n.id) ?? []));
    if (page?.next_cursor === undefined) break;
    cursor = page.next_cursor;
  }
  requests.length = 0;
  await data.queryClient.infiniteQuery(notificationsQuery(data, "all"));
  await data.queryClient.query(notificationMarkerQuery(data));
  const id = (n: number) => all.at(-n) ?? `none of ${String(n)}`;
  return { fake, data, requests, id, moves: () => moved.seen.map((s) => s.body.last_read_id) };
}

/** What the list and the shell read now. */
const read = (data: AppData): { reads: NotificationReads; count: number | undefined } =>
  probe(data, () => ({ reads: useNotificationReads(), count: useUnreadNotifications() }));

/** Waits for the marker's moves the reads started. */
const settle = () => Bun.sleep(5);

test("a notification newer than the marker is unread, and the count is the server's", async () => {
  const { data, id } = await server(5, 3);
  const { reads, count } = read(data);
  expect([5, 4, 3, 2, 1].map((n) => reads.unread(id(n)))).toEqual([true, true, true, false, false]);
  expect(count).toBe(3);
});

test("reading the newest first leaves the marker until the oldest unread one is read, then moves it to the top", async () => {
  const { data, id, moves } = await server(5, 3);
  read(data).reads.read(id(5));
  read(data).reads.read(id(4));
  await settle();
  expect(moves()).toEqual([]);
  const now = read(data);
  expect(now.reads.unread(id(5))).toBe(false);
  expect(now.reads.unread(id(3))).toBe(true);
  expect(now.count).toBe(1);
  now.reads.read(id(3));
  await settle();
  expect(moves()).toEqual([id(5)]);
  expect(read(data).count).toBe(0);
});

test("reading the oldest unread one alone moves the marker to it", async () => {
  const { data, id, moves } = await server(5, 3);
  read(data).reads.read(id(3));
  await settle();
  expect(moves()).toEqual([id(3)]);
  expect(read(data).count).toBe(2);
});

test("while the loaded list does not reach the marker, the marker stays", async () => {
  // The first page holds the 20 newest of 25, all unread, and the marker is under them.
  const { data, id, moves } = await server(25, 23);
  for (const n of [25, 24]) read(data).reads.read(id(n));
  await settle();
  expect(moves()).toEqual([]);
  expect(read(data).count).toBe(21);
});

test("a notification at or under the marker is read already, and reading it asks nothing", async () => {
  const { data, id, moves } = await server(3, 1);
  read(data).reads.read(id(1));
  await settle();
  expect(moves()).toEqual([]);
});

test("reading all moves the marker to the newest notification", async () => {
  const { data, id, moves } = await server(3, 3);
  await read(data).reads.readAll();
  expect(moves()).toEqual([id(3)]);
  expect(read(data).count).toBe(0);
});

test("a notification from the stream goes to the top of the lists it belongs to, unread", async () => {
  const { fake, data, id } = await server(2, 1);
  await data.queryClient.infiniteQuery(notificationsQuery(data, "mentions"));
  const mention = { ...fake.newNotification(), type: "mention" as const };
  const favourite = fake.newNotification();
  receiveNotification(data, mention);
  receiveNotification(data, favourite);
  const ids = (filter: "all" | "mentions") =>
    data.queryClient
      .getQueryData(notificationsQuery(data, filter).queryKey)
      ?.pages.flatMap((p) => p.items.map((n) => n.id));
  expect(ids("all")).toEqual([favourite.id, mention.id, id(2), id(1)]);
  expect(ids("mentions")?.[0]).toBe(mention.id);
  expect(ids("mentions")).not.toContain(favourite.id);
  const { reads, count } = read(data);
  expect(reads.unread(favourite.id)).toBe(true);
  expect(count).toBe(3);
});

test("the Mentions list asks for the mentions, the quotes and the new posts of subscribed accounts", async () => {
  const { fake, data } = fakeData();
  const asked = fake.script("GET /notifications");
  await data.queryClient.infiniteQuery(notificationsQuery(data, "mentions"));
  expect(asked.seen.map((s) => s.url.searchParams.getAll("types").toSorted())).toEqual([
    ["mention", "quote", "status"],
  ]);
});

test("a new post of a subscribed account and a quote from the stream go to the Mentions list", async () => {
  const { fake, data } = await server(2, 1);
  await data.queryClient.infiniteQuery(notificationsQuery(data, "mentions"));
  const arrives = (type: "status" | "quote" | "follow") => {
    const notification = { ...fake.newNotification(), type };
    receiveNotification(data, notification);
    return notification.id;
  };
  const [posted, quoted, followed] = [arrives("status"), arrives("quote"), arrives("follow")];
  const mentions = data.queryClient
    .getQueryData(notificationsQuery(data, "mentions").queryKey)
    ?.pages.flatMap((p) => p.items.map((n) => n.id));
  expect(mentions?.slice(0, 2)).toEqual([quoted, posted]);
  expect(mentions).not.toContain(followed);
});

/** A server with three mentions, which both lists hold, read into both; `ids(filter)` is what a list holds now. */
async function dismissable() {
  const { fake, data, requests, id } = await server(3, 0);
  await data.queryClient.infiniteQuery(notificationsQuery(data, "mentions"));
  requests.length = 0;
  const ids = (filter: "all" | "mentions") =>
    data.queryClient
      .getQueryData(notificationsQuery(data, filter).queryKey)
      ?.pages.flatMap((p) => p.items.map((n) => n.id));
  return { fake, data, requests, id, ids };
}

test("a dismissed notification leaves every list at once, and the server is told", async () => {
  const { fake, data, requests, id, ids } = await dismissable();
  const write = fake.script("POST /notifications/{id}/dismiss", { hold: "response" });
  const done = probe(data, useDismissNotification)(id(2));
  expect(ids("all")).toEqual([id(3), id(1)]);
  expect(ids("mentions")).toEqual([id(3), id(1)]);
  write.release();
  await done;
  expect(requests).toEqual([`POST /notifications/${id(2)}/dismiss`]);
  expect(ids("all")).toEqual([id(3), id(1)]);
  expect(ids("mentions")).toEqual([id(3), id(1)]);
});

test("a dismissal the server refuses puts the notification back where it was in every list, and rejects", async () => {
  const { fake, data, id, ids } = await dismissable();
  const write = fake.script("POST /notifications/{id}/dismiss", { hold: "request", fail: 500 });
  const done = probe(
    data,
    useDismissNotification,
  )(id(2)).then(
    () => "resolved",
    () => "rejected",
  );
  expect(ids("all")).toEqual([id(3), id(1)]);
  write.release();
  expect(await done).toBe("rejected");
  expect(ids("all")).toEqual([id(3), id(2), id(1)]);
  expect(ids("mentions")).toEqual([id(3), id(2), id(1)]);
});
