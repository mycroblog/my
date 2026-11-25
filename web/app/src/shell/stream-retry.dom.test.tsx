// SPDX-License-Identifier: AGPL-3.0-only

// A pull of a timeline on a phone starts a stream the server refused for good again, as the mark of the stream then
// shows; a pull while the stream is open reads the timeline and opens no second stream.

import { expect, test } from "bun:test";
import { act, screen, waitFor, within } from "@testing-library/react";
import { main, renderApp } from "../test/app";
import { fakeApp } from "../test/server";
import { onAPhone } from "../test/viewport";

type Key = "stream_open" | "stream_stopped";

/** The catalog of the app in English, as its translators keep it. */
const catalog: unknown = await Bun.file(new URL("../../locales/en/app.json", import.meta.url)).json();

/** The words of the app's message `app.<key>`, as the catalog has them. */
function message(key: Key): string {
  if (typeof catalog !== "object" || catalog === null) throw new Error("test: no catalog of en");
  const entry: unknown = Reflect.get(catalog, `app.${key}`) ?? Reflect.get(catalog, key);
  const words: unknown = typeof entry === "object" && entry !== null ? Reflect.get(entry, "defaultMessage") : entry;
  if (typeof words !== "string") throw new Error(`test: the catalog of en has no app.${key}`);
  return words;
}

/** A touch event of `type` with one finger at `y` px, or none (an end). */
function touch(target: Element, type: string, y?: number) {
  const e = new Event(type, { bubbles: true });
  const touches = y === undefined ? [] : [{ clientX: 0, clientY: y }];
  Object.defineProperty(e, "touches", { value: touches });
  act(() => {
    target.dispatchEvent(e);
  });
}

/** A finger on the first post of the screen's feed, pulled down well past the threshold from the top, then let go. */
async function pullTheFeed() {
  document.documentElement.scrollTop = 0;
  const [feed] = await main().findAllByRole("feed");
  if (feed === undefined) throw new Error("test: the screen shows no feed");
  const [post] = await within(feed).findAllByRole("article");
  if (post === undefined) throw new Error("test: the feed shows no post");
  touch(post, "touchstart", 0);
  touch(post, "touchmove", 100);
  touch(post, "touchmove", 200);
  touch(post, "touchend");
}

/** The timelines whose pull starts a stopped stream again: home's, and a column's (the public timeline). */
const timelines = [
  ["the home timeline", "/", "GET /timelines/home"],
  ["the public timeline", "/public", "GET /timelines/federated"],
] as const;

for (const [name, at, read] of timelines) {
  test(`a pull of ${name} while the stream is stopped starts it again, and its mark says it is open`, async () => {
    onAPhone();
    const on = fakeApp();
    const streams = on.fake.script("GET /streaming", { times: 1, fail: 403 });
    await renderApp(at, { on });
    expect(await screen.findByRole("img", { name: message("stream_stopped") })).toBeDefined();
    expect(streams.seen).toHaveLength(1);
    const reads = on.fake.script(read);

    await pullTheFeed();

    expect(await screen.findByRole("img", { name: message("stream_open") })).toBeDefined();
    expect(streams.seen).toHaveLength(2);
    await waitFor(() => {
      if (reads.seen.length === 0) throw new Error("the pull did not read the timeline");
    });
  });
}

test("a pull of the home timeline while the stream is open reads the timeline and opens no second stream", async () => {
  onAPhone();
  const on = fakeApp();
  const streams = on.fake.script("GET /streaming");
  await renderApp("/", { on });
  expect(await screen.findByRole("img", { name: message("stream_open") })).toBeDefined();
  expect(streams.seen).toHaveLength(1);
  const reads = on.fake.script("GET /timelines/home");

  await pullTheFeed();

  await waitFor(() => {
    if (reads.seen.length === 0) throw new Error("the pull did not read the timeline");
  });
  await waitFor(() => {
    if (on.data.queryClient.isFetching() > 0) throw new Error("the server's answers are still on their way");
  });
  expect(streams.seen).toHaveLength(1);
  expect(screen.getByRole("img", { name: message("stream_open") })).toBeDefined();
});
