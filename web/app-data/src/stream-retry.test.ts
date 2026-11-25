// SPDX-License-Identifier: AGPL-3.0-only

// Starting a stopped stream again (`retryStream`), as a pull of a timeline does: it does nothing unless the stream
// stopped, reads the stream again in one tab of the browser, and has every tab read its timelines again once it opens.

import { expect, test } from "bun:test";
import { createFakeApi, fakeOrigin, scenario, type FakeApi } from "@my/e2e/fake";
import type { TabGroup } from "@my/platform";
import type { AppData } from "./app-data";
import {
  homeTimelineQuery,
  retryStream,
  startStream,
  streamStateOf,
  watchStreamState,
  type StreamState,
} from "./index";
import { dataOn } from "./test/data";
import { fakeBrowser, fakeStreams, type FakeStream } from "./test/tabs";

/** A server of the scenario where the reader follows sam, whose one post is their home timeline. */
const server = (): FakeApi =>
  createFakeApi({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [{ key: "1", author: "sam", text: "Bread at nine.", visibility: "public", language: "en" }],
    },
  });

/** A tab of a browser with a session on `fake`: its data has a connection to the server, which a stream needs. */
const tab = (fake: FakeApi): AppData => ({ ...dataOn(fake).data, connection: { server: fakeOrigin } });

/** The state of each tab's stream. */
const states = (tabs: AppData[]) => tabs.map((data) => streamStateOf(data));

/** The states a tab's stream goes through from now, each once when it repeats. */
function watched(data: AppData) {
  const seen: (StreamState | undefined)[] = [];
  const stop = watchStreamState(data, () => {
    const now = streamStateOf(data);
    if (seen.at(-1) !== now || seen.length === 0) seen.push(now);
  });
  return { seen, stop };
}

/** Lets what the tabs post to each other and what their streams settle come: a few turns of the event loop. */
const turns = async () => {
  for (let i = 0; i < 5; i += 1) await Bun.sleep(0);
};

/**
 * Starts the stream of each tab, the first tab first, in one browser (or each alone, with `null`), each tab with a fake
 * runStream of its own: `of(i)` is the streams tab `i` read, and `all()` every stream of the browser in its order.
 */
function start(tabs: AppData[], group: (() => TabGroup | null) | null = fakeBrowser().join) {
  const all: FakeStream[] = [];
  const runs = tabs.map(() => fakeStreams());
  const stops = tabs.map((data, i) => {
    const own = runs[i];
    if (own === undefined) throw new Error("test: a tab has no fake runStream");
    return startStream(data, {
      tabs: group === null ? null : group(),
      run: (options) => {
        const ran = own.run(options);
        const made = own.streams.at(-1);
        if (made !== undefined) all.push(made);
        return ran;
      },
    });
  });
  return { all: () => all, of: (i: number) => runs[i]?.streams ?? [], stops };
}

/** The last stream the browser read: the one a test drives next. */
function last(streams: readonly FakeStream[]): FakeStream {
  const stream = streams.at(-1);
  if (stream === undefined) throw new Error("test: the browser read no stream");
  return stream;
}

/** Has the server refuse every stream the browser reads, in turn, until every tab's stream stopped. */
async function refuseAll(tabs: AppData[], all: () => readonly FakeStream[]) {
  for (let i = 0; i < tabs.length + 1 && states(tabs).some((state) => state !== "stopped"); i += 1) {
    last(all()).refuse();
    await turns();
  }
  expect(states(tabs)).toEqual(tabs.map(() => "stopped"));
}

test("a retry does nothing to a stream that is not stopped: connecting, open or reconnecting", async () => {
  const data = tab(server());
  const { all, stops } = start([data], null);
  retryStream(data);
  await turns();
  expect(all()).toHaveLength(1);
  expect(streamStateOf(data)).toBe("connecting");

  last(all()).open(false);
  retryStream(data);
  await turns();
  expect(all()).toHaveLength(1);
  expect(streamStateOf(data)).toBe("open");

  last(all()).drop(new Error("test: the connection broke"));
  retryStream(data);
  await turns();
  expect(all()).toHaveLength(1);
  expect(streamStateOf(data)).toBe("reconnecting");
  expect(last(all()).ended()).toBe(false);
  stops[0]?.();
});

test("a retry does nothing in a tab that never started its stream, nor in one without a session", async () => {
  const fake = server();
  const unstarted = tab(fake);
  retryStream(unstarted);
  await turns();
  expect(streamStateOf(unstarted)).toBeUndefined();

  // A visitor's data has no connection: its stream starts nothing, and a retry neither.
  const visitor = dataOn(fake).data;
  const { all, stops } = start([visitor]);
  retryStream(visitor);
  await turns();
  expect(all()).toHaveLength(0);
  expect(streamStateOf(visitor)).toBeUndefined();
  stops[0]?.();
});

test("a retry does nothing once the tab stopped its stream, though the server had refused it", async () => {
  const data = tab(server());
  const { all, stops } = start([data], null);
  await turns();
  last(all()).refuse();
  await turns();
  expect(streamStateOf(data)).toBe("stopped");

  stops[0]?.();
  retryStream(data);
  await turns();
  expect(all()).toHaveLength(1);
  expect(streamStateOf(data)).toBeUndefined();
});

test("a retry of a stopped stream reads it again: connecting, then open once it opens", async () => {
  const data = tab(server());
  const { all, stops } = start([data], null);
  await turns();
  last(all()).open(false);
  last(all()).refuse();
  await turns();
  expect(streamStateOf(data)).toBe("stopped");
  const { seen } = watched(data);

  retryStream(data);
  await turns();
  expect(all()).toHaveLength(2);
  expect(streamStateOf(data)).toBe("connecting");
  last(all()).open(false);

  expect(streamStateOf(data)).toBe("open");
  expect(seen).toEqual(["connecting", "open"]);
  stops[0]?.();
  expect(last(all()).ended()).toBe(true);
});

test("a retry the server refuses again goes back to stopped, and tries nothing more by itself", async () => {
  const data = tab(server());
  const { all, stops } = start([data], null);
  await turns();
  last(all()).refuse();
  await turns();
  const { seen } = watched(data);

  retryStream(data);
  await turns();
  last(all()).refuse();
  await turns();

  expect(streamStateOf(data)).toBe("stopped");
  expect(seen).toEqual(["connecting", "stopped"]);
  expect(all()).toHaveLength(2);

  // Stopped again, it can be retried again.
  retryStream(data);
  await turns();
  expect(all()).toHaveLength(3);
  last(all()).open(false);
  expect(streamStateOf(data)).toBe("open");
  stops[0]?.();
});

test("a tab refused the lock reads its own stream again on a retry", async () => {
  const data = tab(server());
  const refusing: TabGroup = {
    lead: () => Promise.reject(new DOMException("no storage", "SecurityError")),
    post: () => {},
    listen: () => () => {},
    close: () => {},
  };
  const { all, stops } = start([data], () => refusing);
  await turns();
  expect(all()).toHaveLength(1);
  last(all()).refuse();
  await turns();
  expect(streamStateOf(data)).toBe("stopped");

  retryStream(data);
  await turns();
  expect(all()).toHaveLength(2);
  last(all()).open(false);
  expect(streamStateOf(data)).toBe("open");
  stops[0]?.();
});

test("in a browser of several tabs, a refused stream passes to each waiting tab in turn, then stays stopped", async () => {
  const fake = server();
  const tabs = [tab(fake), tab(fake), tab(fake)];
  const { all, stops } = start(tabs);
  await turns();
  expect(all()).toHaveLength(1);
  last(all()).open(false);

  await refuseAll(tabs, all);
  // Each tab read it once, and none reads it again by itself.
  expect(all()).toHaveLength(3);
  await turns();
  expect(all()).toHaveLength(3);
  for (const stop of stops) stop();
});

test("a retry in any tab of a stopped browser reads one stream again, which every tab shows", async () => {
  const fake = server();
  const tabs: [AppData, AppData, AppData] = [tab(fake), tab(fake), tab(fake)];
  const { all, stops } = start(tabs);
  await turns();
  await refuseAll(tabs, all);
  const before = all().length;

  retryStream(tabs[1]);
  await turns();
  expect(all()).toHaveLength(before + 1);
  expect(states(tabs)).toEqual(["connecting", "connecting", "connecting"]);
  last(all()).open(false);
  await turns();
  expect(states(tabs)).toEqual(["open", "open", "open"]);

  // The other tabs are no longer stopped: their retries read nothing more.
  retryStream(tabs[0]);
  retryStream(tabs[2]);
  await turns();
  expect(all()).toHaveLength(before + 1);
  for (const stop of stops) stop();
});

test("retries in every tab of a stopped browser at once read one stream, and the others wait behind it", async () => {
  const fake = server();
  const tabs = [tab(fake), tab(fake), tab(fake)];
  const { all, of, stops } = start(tabs);
  await turns();
  await refuseAll(tabs, all);
  const before = tabs.map((_, i) => of(i).length);

  for (const data of tabs) retryStream(data);
  await turns();
  const leaders = tabs.flatMap((_, i) => (of(i).length > (before[i] ?? 0) ? [i] : []));
  expect(leaders).toHaveLength(1);
  const [leader] = leaders;
  if (leader === undefined) throw new Error("test: no tab reads the stream again");
  last(of(leader)).open(false);
  await turns();
  expect(states(tabs)).toEqual(["open", "open", "open"]);

  // The tab that reads it closes: one of the tabs that waited takes it over, and the browser still reads one stream.
  stops[leader]?.();
  await turns();
  expect(last(of(leader)).ended()).toBe(true);
  const next = tabs.flatMap((_, i) => (i !== leader && of(i).length > (before[i] ?? 0) ? [i] : []));
  expect(next).toHaveLength(1);
  const [taker] = next;
  if (taker === undefined) throw new Error("test: no tab took the stream over");
  last(of(taker)).open(false);
  await turns();
  expect(states(tabs.filter((_, i) => i !== leader))).toEqual(["open", "open"]);
  for (const stop of stops) stop();
});

/** A tab whose home timeline is read, and whether it marked it to be read again since. */
async function reading(data: AppData) {
  await data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });
  return () => data.queryClient.getQueryState(homeTimelineQuery(data).queryKey)?.isInvalidated === true;
}

/** Reads the tab's home timeline again, so that it is no longer marked to be read again. */
const readAgain = (data: AppData) => data.queryClient.refetchQueries({ queryKey: homeTimelineQuery(data).queryKey });

test("once a retried stream opens, the tab reads its timelines again, and not before", async () => {
  const data = tab(server());
  const caughtUp = await reading(data);
  const { all, stops } = start([data], null);
  await turns();
  last(all()).open(false);
  last(all()).refuse();
  await turns();
  await readAgain(data);
  expect(caughtUp()).toBe(false);

  retryStream(data);
  await turns();
  expect(all()).toHaveLength(2);
  expect(caughtUp()).toBe(false);
  last(all()).open(false);
  expect(caughtUp()).toBe(true);
  stops[0]?.();
});

test("once a retried stream opens, every tab of the browser reads its timelines again", async () => {
  const fake = server();
  const tabs: [AppData, AppData, AppData] = [tab(fake), tab(fake), tab(fake)];
  const caughtUp = await Promise.all(tabs.map((data) => reading(data)));
  const { all, stops } = start(tabs);
  await turns();
  last(all()).open(false);
  await refuseAll(tabs, all);
  await Promise.all(tabs.map((data) => readAgain(data)));
  expect(caughtUp.map((was) => was())).toEqual([false, false, false]);
  const before = all().length;

  retryStream(tabs[2]);
  await turns();
  expect(all()).toHaveLength(before + 1);
  expect(caughtUp.map((was) => was())).toEqual([false, false, false]);
  last(all()).open(false);
  await turns();
  expect(caughtUp.map((was) => was())).toEqual([true, true, true]);
  for (const stop of stops) stop();
});
