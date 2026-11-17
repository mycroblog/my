// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { createFakeApi, fakeOrigin, scenario, type FakeApi } from "@my/e2e/fake";
import type { StreamEvent } from "@mycroblog/mykit";
import type { TabGroup } from "@my/platform";
import type { AppData } from "./app-data";
import { notificationMarkerQuery } from "./notifications";
import { shadowed } from "./shadow";
import { startStream } from "./stream";
import { dataOn } from "./test/data";
import { fakeBrowser, fakeStreams } from "./test/tabs";
import { homeTimelineQuery } from "./timeline";

/**
 * A server where the reader follows sam, whose one post is their home timeline, and two people followed the reader,
 * which they have not read yet; `one` is the post's ID, and `update()` and `notification()` are events of the
 * server's stream: a new post of sam's, and a new notification.
 */
function server() {
  const fake = createFakeApi({
    seed: {
      people: scenario.people,
      follows: [
        { follower: "yuki", followee: "sam" },
        { follower: "marco", followee: "yuki" },
        { follower: "mio", followee: "yuki" },
      ],
      posts: [{ key: "1", author: "sam", text: "Bread at nine.", visibility: "public", language: "en" }],
    },
  });
  const update = (): StreamEvent & { type: "update" } => {
    const [status] = fake.newPosts(1);
    if (status === undefined) throw new Error("test: the fake made no post");
    return { type: "update", status };
  };
  const notification = (): StreamEvent => ({ type: "notification", notification: fake.newNotification() });
  return { fake, one: fake.idOf("1"), update, notification };
}

/** A tab of a browser with a session on `fake`: its home timeline and its unread count (2) are read. */
async function tab(fake: FakeApi) {
  const made = dataOn(fake);
  const data: AppData = { ...made.data, connection: { server: fakeOrigin } };
  await data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });
  await data.queryClient.query(notificationMarkerQuery(data));
  return { data, requests: made.requests };
}

/** The IDs of the statuses that show on each page of the tab's home timeline. */
const home = (data: AppData) =>
  data.queryClient
    .getQueryData(homeTimelineQuery(data).queryKey)
    ?.pages.map((p) => p.items.filter((s) => shadowed(data, s) !== undefined).map((s) => s.id));
const unread = (data: AppData) =>
  data.queryClient.getQueryData<{ unread_count: number }>(notificationMarkerQuery(data).queryKey)?.unread_count;
/** Whether the tab marked its home timeline and its unread count to be read again. */
const caughtUp = (data: AppData) =>
  data.queryClient.getQueryState(homeTimelineQuery(data).queryKey)?.isInvalidated === true &&
  data.queryClient.getQueryState(notificationMarkerQuery(data).queryKey)?.isInvalidated === true;

/** Starts the stream of each tab in one browser, the first tab first. */
function start(tabs: AppData[], group: (() => TabGroup | null) | null = fakeBrowser().join) {
  const { run, streams } = fakeStreams();
  const stops = tabs.map((data) => startStream(data, { tabs: group === null ? null : group(), run }));
  return { streams, stops };
}

test("one tab of the browser reads the stream, and every tab receives its events", async () => {
  const { fake, one, update, notification } = server();
  const [a, b, c] = [await tab(fake), await tab(fake), await tab(fake)];
  for (const { data } of [a, b, c]) expect([home(data), unread(data)]).toEqual([[[one]], 2]);
  const { streams } = start([a.data, b.data, c.data]);
  await Bun.sleep(0);
  expect(streams).toHaveLength(1);
  const [stream] = streams;
  stream?.open(false);
  const two = update();
  stream?.send(two);
  stream?.send(notification());
  for (const { data } of [a, b, c]) {
    expect(home(data)).toEqual([[two.status.id, one]]);
    expect(unread(data)).toBe(3);
    expect(caughtUp(data)).toBe(false);
  }
});

test("a status deleted on the stream leaves the timelines of every tab", async () => {
  const { fake, one, update } = server();
  const [a, b] = [await tab(fake), await tab(fake)];
  const { streams } = start([a.data, b.data]);
  await Bun.sleep(0);
  const [stream] = streams;
  stream?.open(false);
  const two = update();
  stream?.send(two);
  stream?.send({ type: "delete", id: one });
  for (const { data } of [a, b]) expect(home(data)).toEqual([[two.status.id]]);
});

test("when the leading tab closes, another takes the stream over and every tab catches up", async () => {
  const { fake, one, update } = server();
  const [a, b, c] = [await tab(fake), await tab(fake), await tab(fake)];
  const { streams, stops } = start([a.data, b.data, c.data]);
  await Bun.sleep(0);
  streams[0]?.open(false);
  stops[0]?.();
  await Bun.sleep(0);
  expect(streams[0]?.ended()).toBe(true);
  expect(streams).toHaveLength(2);
  streams[1]?.open(false);
  expect(caughtUp(b.data)).toBe(true);
  expect(caughtUp(c.data)).toBe(true);
  const two = update();
  streams[1]?.send(two);
  expect(home(c.data)?.[0]).toContain(two.status.id);
  expect(home(a.data)).toEqual([[one]]);
});

test("after the stream reconnects, every tab catches up", async () => {
  const { fake } = server();
  const [a, b] = [await tab(fake), await tab(fake)];
  const { streams } = start([a.data, b.data]);
  await Bun.sleep(0);
  streams[0]?.open(false);
  expect(caughtUp(a.data) || caughtUp(b.data)).toBe(false);
  streams[0]?.open(true);
  expect(caughtUp(a.data)).toBe(true);
  expect(caughtUp(b.data)).toBe(true);
});

test("a message of another version of the client that it cannot read is left alone", async () => {
  const { fake, one, update } = server();
  const [a, b] = [await tab(fake), await tab(fake)];
  const browser = fakeBrowser();
  start([b.data], browser.join);
  const other = browser.join();
  other.post({ kind: "something new" });
  other.post({ kind: "event", event: null });
  const two = update();
  other.post({ kind: "event", event: two });
  expect(home(b.data)).toEqual([[two.status.id, one]]);
  expect(home(a.data)).toEqual([[one]]);
});

test("without Web Locks or BroadcastChannel, each tab reads a stream of its own", async () => {
  const { fake, one, update } = server();
  const [a, b] = [await tab(fake), await tab(fake)];
  const { streams } = start([a.data, b.data], null);
  expect(streams).toHaveLength(2);
  const two = update();
  streams[0]?.send(two);
  expect(home(a.data)).toEqual([[two.status.id, one]]);
  expect(home(b.data)).toEqual([[one]]);
});

test("a browser that refuses the lock reads a stream in the tab", async () => {
  const { data } = await tab(server().fake);
  const refusing: TabGroup = {
    lead: () => Promise.reject(new DOMException("no storage", "SecurityError")),
    post: () => {},
    listen: () => () => {},
    close: () => {},
  };
  const { run, streams } = fakeStreams();
  startStream(data, { tabs: refusing, run });
  await Bun.sleep(0);
  expect(streams).toHaveLength(1);
});

test("without a session there is no stream", () => {
  const { data } = dataOn(server().fake);
  const { run, streams } = fakeStreams();
  startStream(data, { tabs: fakeBrowser().join(), run })();
  expect(streams).toHaveLength(0);
});

test("a stream the server refused for good lets the next tab try", async () => {
  const { fake } = server();
  const [a, b] = [await tab(fake), await tab(fake)];
  const browser = fakeBrowser();
  const tried: string[] = [];
  const refused = (name: string) => () => {
    tried.push(name);
    return Promise.reject(new Error("401"));
  };
  startStream(a.data, { tabs: browser.join(), run: refused("a") });
  startStream(b.data, { tabs: browser.join(), run: refused("b") });
  await Bun.sleep(0);
  expect(tried).toEqual(["a", "b"]);
});
