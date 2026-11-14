// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import type { StreamEvent } from "@mycroblog/mykit";
import type { TabGroup } from "@my/platform";
import type { AppData } from "./app-data";
import { notificationMarkerQuery } from "./notifications";
import { startStream } from "./stream";
import { fakeAppData, json, page, status } from "./test/fake";
import { fakeBrowser, fakeStreams } from "./test/tabs";
import { homeTimelineQuery } from "./timeline";

/** A tab with a session: its home timeline holds ["1"] and its unread count is 2, both read from the server. */
async function tab() {
  const fake = fakeAppData((request) =>
    new URL(request.url).pathname.endsWith("/notifications/marker")
      ? json(200, { last_read_id: "n0", unread_count: 2 })
      : json(200, page(["1"])),
  );
  const data: AppData = { ...fake.data, connection: { server: "https://example.com" } };
  await data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });
  await data.queryClient.query(notificationMarkerQuery(data));
  return { data, requests: fake.requests };
}

const home = (data: AppData) =>
  data.queryClient.getQueryData(homeTimelineQuery(data).queryKey)?.pages.map((p) => p.items.map((s) => s.id));
const unread = (data: AppData) =>
  data.queryClient.getQueryData<{ unread_count: number }>(notificationMarkerQuery(data).queryKey)?.unread_count;
/** Whether the tab marked its home timeline and its unread count to be read again. */
const caughtUp = (data: AppData) =>
  data.queryClient.getQueryState(homeTimelineQuery(data).queryKey)?.isInvalidated === true &&
  data.queryClient.getQueryState(notificationMarkerQuery(data).queryKey)?.isInvalidated === true;

const update = (id: string): StreamEvent => ({ type: "update", status: status(id) });
const notification: StreamEvent = { type: "notification", notification: { id: "n1" } as never };

/** Starts the stream of each tab in one browser, the first tab first. */
function start(tabs: AppData[], group: (() => TabGroup | null) | null = fakeBrowser().join) {
  const { run, streams } = fakeStreams();
  const stops = tabs.map((data) => startStream(data, { tabs: group === null ? null : group(), run }));
  return { streams, stops };
}

test("one tab of the browser reads the stream, and every tab receives its events", async () => {
  const [a, b, c] = [await tab(), await tab(), await tab()];
  const { streams } = start([a.data, b.data, c.data]);
  await Bun.sleep(0);
  expect(streams).toHaveLength(1);
  const [stream] = streams;
  stream?.open(false);
  stream?.send(update("2"));
  stream?.send(notification);
  for (const { data } of [a, b, c]) {
    expect(home(data)).toEqual([["2", "1"]]);
    expect(unread(data)).toBe(3);
    expect(caughtUp(data)).toBe(false);
  }
});

test("when the leading tab closes, another takes the stream over and every tab catches up", async () => {
  const [a, b, c] = [await tab(), await tab(), await tab()];
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
  streams[1]?.send(update("2"));
  expect(home(c.data)?.[0]).toContain("2");
  expect(home(a.data)).toEqual([["1"]]);
});

test("after the stream reconnects, every tab catches up", async () => {
  const [a, b] = [await tab(), await tab()];
  const { streams } = start([a.data, b.data]);
  await Bun.sleep(0);
  streams[0]?.open(false);
  expect(caughtUp(a.data) || caughtUp(b.data)).toBe(false);
  streams[0]?.open(true);
  expect(caughtUp(a.data)).toBe(true);
  expect(caughtUp(b.data)).toBe(true);
});

test("a message of another version of the client that it cannot read is left alone", async () => {
  const [a, b] = [await tab(), await tab()];
  const browser = fakeBrowser();
  start([b.data], browser.join);
  const other = browser.join();
  other.post({ kind: "something new" });
  other.post({ kind: "event", event: null });
  other.post({ kind: "event", event: update("2") });
  expect(home(b.data)).toEqual([["2", "1"]]);
  expect(home(a.data)).toEqual([["1"]]);
});

test("without Web Locks or BroadcastChannel, each tab reads a stream of its own", async () => {
  const [a, b] = [await tab(), await tab()];
  const { streams } = start([a.data, b.data], null);
  expect(streams).toHaveLength(2);
  streams[0]?.send(update("2"));
  expect(home(a.data)).toEqual([["2", "1"]]);
  expect(home(b.data)).toEqual([["1"]]);
});

test("a browser that refuses the lock reads a stream in the tab", async () => {
  const { data } = await tab();
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
  const { data } = fakeAppData(() => json(200, {}));
  const { run, streams } = fakeStreams();
  startStream(data, { tabs: fakeBrowser().join(), run })();
  expect(streams).toHaveLength(0);
});

test("a stream the server refused for good lets the next tab try", async () => {
  const [a, b] = [await tab(), await tab()];
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
