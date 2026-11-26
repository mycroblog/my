// SPDX-License-Identifier: AGPL-3.0-only

// The stream of a public timeline (ADR-0013 §17), shared by the tabs that show it as the session's stream is shared by
// every tab (ADR-0028 §18): one tab reads it, every tab that shows the timeline receives its posts, it closes once no
// tab shows the timeline, and the timeline is read again whenever the stream opens.

import { expect, test } from "bun:test";
import { createFakeApi, fakeOrigin, scenario, type FakeApi } from "@my/e2e/fake";
import type { StreamEvent } from "@mycroblog/mykit";
import type { TabGroup } from "@my/platform";
import type { AppData } from "./app-data";
import { retryStream } from "./stream-retry";
import { shadowed } from "./shadow";
import { startTimelineStream } from "./stream";
import { dataOn } from "./test/data";
import { fakeBrowser, fakeStreams } from "./test/tabs";
import { publicTimelineQuery } from "./timeline";

/** A server whose local timeline has sam's one post; `update()` is an event of its stream: a new post. */
function server() {
  const fake = createFakeApi({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [{ key: "1", author: "sam", text: "Bread at nine.", visibility: "public", language: "en" }],
    },
  });
  const update = (): StreamEvent & { type: "update" } => {
    const [status] = fake.newPosts(1);
    if (status === undefined) throw new Error("test: the fake made no post");
    return { type: "update", status };
  };
  return { fake, one: fake.idOf("1"), update };
}

/** A tab of a browser with a session on `fake`, whose local timeline is read. */
async function tab(fake: FakeApi): Promise<AppData> {
  const data: AppData = { ...dataOn(fake).data, connection: { server: fakeOrigin } };
  await data.queryClient.infiniteQuery({ ...publicTimelineQuery(data, "local"), staleTime: Infinity });
  return data;
}

/** The IDs of the statuses that show on each page of the tab's local timeline. */
const local = (data: AppData) =>
  data.queryClient
    .getQueryData(publicTimelineQuery(data, "local").queryKey)
    ?.pages.map((p) => p.items.filter((s) => shadowed(data, s) !== undefined).map((s) => s.id));

/** Whether the tab marked its local timeline to be read again. */
const readAgain = (data: AppData) =>
  data.queryClient.getQueryState(publicTimelineQuery(data, "local").queryKey)?.isInvalidated === true;

/** Marks the tab's local timeline as read, so that a later break shows. */
const markRead = (data: AppData) => {
  data.queryClient.getQueryCache().find({ queryKey: publicTimelineQuery(data, "local").queryKey })?.setState({
    isInvalidated: false,
  });
};

/** Starts the stream of the local timeline in each tab of one browser, the first tab first. */
function start(tabs: AppData[], group: (() => TabGroup | null) | null = fakeBrowser().join) {
  const { run, streams } = fakeStreams();
  const stops = tabs.map((data) => startTimelineStream(data, "local", { tabs: group === null ? null : group(), run }));
  return { streams, stops };
}

test("one tab reads the local timeline's stream, and every tab that shows it receives its posts", async () => {
  const { fake, one, update } = server();
  const [a, b] = [await tab(fake), await tab(fake)];
  const { streams } = start([a, b]);
  await Bun.sleep(0);
  expect(streams).toHaveLength(1);
  expect(streams[0]?.options.timeline).toBe("local");
  streams[0]?.open(false);
  const two = update();
  streams[0]?.send(two);
  for (const data of [a, b]) expect(local(data)?.[0]).toEqual([two.status.id, one]);
});

test("the timeline is read again when its stream first opens, since posts may have come before it", async () => {
  const { fake } = server();
  const data = await tab(fake);
  const { streams } = start([data]);
  await Bun.sleep(0);
  expect(readAgain(data)).toBe(false);
  streams[0]?.open(false);
  expect(readAgain(data)).toBe(true);
});

test("the stream closes once no tab shows the timeline, and stays open while one does", async () => {
  const { fake } = server();
  const [a, b] = [await tab(fake), await tab(fake)];
  const { streams, stops } = start([a, b]);
  await Bun.sleep(0);
  streams[0]?.open(false);
  stops[0]?.();
  await Bun.sleep(0);
  expect(streams[0]?.ended()).toBe(true);
  expect(streams).toHaveLength(2);
  expect(streams[1]?.ended()).toBe(false);
  stops[1]?.();
  await Bun.sleep(0);
  expect(streams[1]?.ended()).toBe(true);
  expect(streams).toHaveLength(2);
});

test("when another tab takes the stream over, the tabs read the timeline again", async () => {
  const { fake } = server();
  const [a, b] = [await tab(fake), await tab(fake)];
  const { streams, stops } = start([a, b]);
  await Bun.sleep(0);
  streams[0]?.open(false);
  markRead(b);
  stops[0]?.();
  await Bun.sleep(0);
  streams[1]?.open(false);
  expect(readAgain(b)).toBe(true);
});

test("after the stream reconnects, the tabs read the timeline again", async () => {
  const { fake } = server();
  const [a, b] = [await tab(fake), await tab(fake)];
  const { streams } = start([a, b]);
  await Bun.sleep(0);
  streams[0]?.open(false);
  markRead(a);
  markRead(b);
  streams[0]?.open(true);
  expect(readAgain(a)).toBe(true);
  expect(readAgain(b)).toBe(true);
});

test("a status deleted on the stream leaves the timeline", async () => {
  const { fake, one, update } = server();
  const data = await tab(fake);
  const { streams } = start([data]);
  await Bun.sleep(0);
  streams[0]?.open(false);
  const two = update();
  streams[0]?.send(two);
  streams[0]?.send({ type: "delete", id: one });
  expect(local(data)?.[0]).toEqual([two.status.id]);
});

test("a pull starts a stream the server refused again", async () => {
  const { fake } = server();
  const data = await tab(fake);
  const { streams } = start([data], null);
  streams[0]?.refuse();
  await Bun.sleep(0);
  expect(streams).toHaveLength(1);
  retryStream(data);
  await Bun.sleep(0);
  expect(streams).toHaveLength(2);
  expect(streams[1]?.options.timeline).toBe("local");
});

test("a visitor has no stream", () => {
  const data = dataOn(server().fake).data;
  const { run, streams } = fakeStreams();
  startTimelineStream(data, "local", { tabs: fakeBrowser().join(), run })();
  expect(streams).toHaveLength(0);
});
