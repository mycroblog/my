// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { createFakeApi, fakeOrigin, scenario, type FakeApi } from "@my/e2e/fake";
import type { TabGroup } from "@my/platform";
import type { AppData } from "./app-data";
import { startStream, streamStateOf, useStreamState, watchStreamState, type StreamState } from "./index";
import { dataOn } from "./test/data";
import { probe } from "./test/probe";
import { fakeBrowser, fakeStreams } from "./test/tabs";

/** A server of the scenario, which every tab of a test reads. */
const server = (): FakeApi => createFakeApi({ seed: { people: scenario.people } });

/** A tab of a browser with a session on `fake`: its data has a connection to the server, which a stream needs. */
const tab = (fake: FakeApi): AppData => ({ ...dataOn(fake).data, connection: { server: fakeOrigin } });

/** Starts the stream of each tab in one browser (or each alone, with `null`), the first tab first. */
function start(tabs: AppData[], group: (() => TabGroup | null) | null = fakeBrowser().join) {
  const { run, streams } = fakeStreams();
  const stops = tabs.map((data) => startStream(data, { tabs: group === null ? null : group(), run }));
  return { streams, stops };
}

/** The state of each tab's stream. */
const states = (tabs: AppData[]) => tabs.map((data) => streamStateOf(data));

/** The states a tab's stream went through from now, each once when it repeats. */
function watched(data: AppData) {
  const seen: (StreamState | undefined)[] = [];
  const stop = watchStreamState(data, () => {
    const now = streamStateOf(data);
    if (seen.at(-1) !== now || seen.length === 0) seen.push(now);
  });
  return { seen, stop };
}

test("a tab that has not started its stream, or has no connection to a server, has no state of it", () => {
  const fake = server();
  expect(streamStateOf(tab(fake))).toBeUndefined();

  // The data of a visitor's app has no connection: starting its stream starts nothing.
  const visitor = dataOn(fake).data;
  const { run, streams } = fakeStreams();
  const stop = startStream(visitor, { tabs: fakeBrowser().join(), run });
  expect(streams).toHaveLength(0);
  expect(streamStateOf(visitor)).toBeUndefined();
  stop();
});

test("a tab's stream is connecting once started, open on each open, and reconnecting between connections", async () => {
  const data = tab(server());
  const { seen } = watched(data);
  const { streams, stops } = start([data]);
  expect(streamStateOf(data)).toBe("connecting");
  await Bun.sleep(0);
  expect(streams).toHaveLength(1);
  const [stream] = streams;

  stream?.open(false);
  expect(streamStateOf(data)).toBe("open");
  // A connection that failed: runStream reports the failure, then waits before the next.
  stream?.drop(new Error("test: the connection broke"));
  expect(streamStateOf(data)).toBe("reconnecting");
  stream?.open(true);
  expect(streamStateOf(data)).toBe("open");
  // A stream the server ended: runStream waits before the next, with no failure to report.
  stream?.drop();
  expect(streamStateOf(data)).toBe("reconnecting");
  stream?.open(true);
  expect(streamStateOf(data)).toBe("open");

  expect(seen).toEqual(["connecting", "open", "reconnecting", "open", "reconnecting", "open"]);
  stops[0]?.();
});

test("a tab's stream that it stopped has no state, and its watchers hear so", async () => {
  const data = tab(server());
  const { seen } = watched(data);
  const { streams, stops } = start([data]);
  await Bun.sleep(0);
  streams[0]?.open(false);

  stops[0]?.();
  expect(streams[0]?.ended()).toBe(true);
  expect(streamStateOf(data)).toBeUndefined();
  expect(seen).toEqual(["connecting", "open", undefined]);
});

test("a watcher that stopped watching hears nothing more", async () => {
  const data = tab(server());
  const { seen, stop } = watched(data);
  const { streams, stops } = start([data]);
  await Bun.sleep(0);
  streams[0]?.open(false);
  stop();
  streams[0]?.drop(new Error("test: the connection broke"));
  streams[0]?.open(true);

  expect(seen).toEqual(["connecting", "open"]);
  expect(streamStateOf(data)).toBe("open");
  stops[0]?.();
});

test("a stream the server refused for good is stopped", async () => {
  const data = tab(server());
  const { streams, stops } = start([data], null);
  await Bun.sleep(0);
  streams[0]?.open(false);
  streams[0]?.refuse();
  await Bun.sleep(0);

  expect(streamStateOf(data)).toBe("stopped");
  // Nothing tries the stream again in a tab alone.
  expect(streams).toHaveLength(1);
  stops[0]?.();
});

test("every tab of the browser shows the state of the stream the leading tab reads", async () => {
  const fake = server();
  const tabs = [tab(fake), tab(fake), tab(fake)];
  const { streams, stops } = start(tabs);
  expect(states(tabs)).toEqual(["connecting", "connecting", "connecting"]);
  await Bun.sleep(0);
  expect(streams).toHaveLength(1);
  const [stream] = streams;

  stream?.open(false);
  await Bun.sleep(0);
  expect(states(tabs)).toEqual(["open", "open", "open"]);
  stream?.drop(new Error("test: the connection broke"));
  await Bun.sleep(0);
  expect(states(tabs)).toEqual(["reconnecting", "reconnecting", "reconnecting"]);
  stream?.open(true);
  await Bun.sleep(0);
  expect(states(tabs)).toEqual(["open", "open", "open"]);
  for (const stop of stops) stop();
});

test("a tab that joins the browser shows connecting while the leading tab has not answered", async () => {
  const fake = server();
  const browser = fakeBrowser();
  const { run, streams } = fakeStreams();
  // A tab that holds the lead and never answers: an older client, or one busy loading.
  const silent = new AbortController();
  void browser.join().lead(silent.signal, () => new Promise<void>(() => undefined));

  const joining = tab(fake);
  const stop = startStream(joining, { tabs: browser.join(), run });
  await Bun.sleep(0);
  expect(streamStateOf(joining)).toBe("connecting");
  // It reads no stream of its own while another tab leads.
  expect(streams).toHaveLength(0);
  stop();
  silent.abort();
});

test("a tab that joins the browser shows the state of the leading tab's stream once it answers", async () => {
  const fake = server();
  const browser = fakeBrowser();
  const { run, streams } = fakeStreams();
  const first = tab(fake);
  const stopFirst = startStream(first, { tabs: browser.join(), run });
  await Bun.sleep(0);
  streams[0]?.drop(new Error("test: the connection broke"));
  expect(streamStateOf(first)).toBe("reconnecting");

  const joining = tab(fake);
  const stopJoining = startStream(joining, { tabs: browser.join(), run });
  await Bun.sleep(0);
  expect(streamStateOf(joining)).toBe("reconnecting");
  expect(streams).toHaveLength(1);
  stopJoining();
  stopFirst();
});

test("when the leading tab closes, the tab that takes the stream over connects, and every tab shows it", async () => {
  const fake = server();
  const tabs = [tab(fake), tab(fake), tab(fake)];
  const { streams, stops } = start(tabs);
  await Bun.sleep(0);
  streams[0]?.open(false);
  await Bun.sleep(0);
  expect(states(tabs)).toEqual(["open", "open", "open"]);

  stops[0]?.();
  await Bun.sleep(0);
  expect(streams).toHaveLength(2);
  // The closed tab has no stream any more.
  expect(states(tabs)).toEqual([undefined, "connecting", "connecting"]);
  streams[1]?.open(false);
  await Bun.sleep(0);
  expect(states(tabs)).toEqual([undefined, "open", "open"]);
  for (const stop of stops) stop();
});

test("when the server refuses the leading tab's stream for good, a waiting tab takes it over and connects", async () => {
  const fake = server();
  const tabs = [tab(fake), tab(fake)];
  const { streams, stops } = start(tabs);
  await Bun.sleep(0);
  streams[0]?.open(false);
  await Bun.sleep(0);
  expect(states(tabs)).toEqual(["open", "open"]);

  streams[0]?.refuse();
  await Bun.sleep(0);
  expect(streams).toHaveLength(2);
  expect(states(tabs)).toEqual(["connecting", "connecting"]);
  streams[1]?.open(false);
  await Bun.sleep(0);
  expect(states(tabs)).toEqual(["open", "open"]);
  for (const stop of stops) stop();
});

test("without Web Locks or BroadcastChannel, each tab shows the state of its own stream", async () => {
  const fake = server();
  const tabs = [tab(fake), tab(fake)];
  const { streams, stops } = start(tabs, null);
  await Bun.sleep(0);
  expect(streams).toHaveLength(2);
  streams[0]?.open(false);
  expect(states(tabs)).toEqual(["open", "connecting"]);
  streams[1]?.open(false);
  streams[0]?.drop(new Error("test: the connection broke"));
  expect(states(tabs)).toEqual(["reconnecting", "open"]);
  for (const stop of stops) stop();
});

test("the hook of the stream's state returns the tab's state as it is now", async () => {
  const data = tab(server());
  expect(probe(data, useStreamState)).toBeUndefined();
  const { streams, stops } = start([data], null);
  expect(probe(data, useStreamState)).toBe("connecting");
  await Bun.sleep(0);
  streams[0]?.open(false);
  expect(probe(data, useStreamState)).toBe("open");
  streams[0]?.refuse();
  await Bun.sleep(0);
  expect(probe(data, useStreamState)).toBe("stopped");
  stops[0]?.();
  expect(probe(data, useStreamState)).toBeUndefined();
});
