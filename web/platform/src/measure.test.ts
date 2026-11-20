// SPDX-License-Identifier: AGPL-3.0-only

// The browser's side of the UX measurement (ADR-0167 §1): the largest contentful paint of the load, once, the
// interactions that took longer than the threshold, and the long animation frames, each read from a performance
// observer the test drives; the moment the page is hidden; and a report that outlives the page.

import { afterEach, expect, spyOn, test } from "bun:test";
import { sendReport, watchPageHidden, watchVitals, type TimedEntry } from "./measure";

/** The entry types of the browser the test runs as. */
const allTypes = ["largest-contentful-paint", "event", "long-animation-frame"];

/**
 * A performance timeline the test fills: `give` delivers entries of a type to its observers, `hold` keeps them. As a
 * browser does, it gives an observer of `event` only the entries as long as its `durationThreshold` (104 ms when it
 * names none).
 */
function fakeTimeline(supported: readonly string[] = allTypes) {
  const observers: FakeObserver[] = [];
  class FakeObserver {
    static readonly supportedEntryTypes = supported;
    readonly types: string[] = [];
    eventThreshold = 104;
    held: TimedEntry[] = [];
    disconnected = false;
    readonly callback: (list: { getEntries(): readonly TimedEntry[] }) => void;
    constructor(callback: (list: { getEntries(): readonly TimedEntry[] }) => void) {
      this.callback = callback;
      observers.push(this);
    }
    observe({ type, durationThreshold }: { type: string; durationThreshold?: number }): void {
      this.types.push(type);
      if (type === "event" && durationThreshold !== undefined) this.eventThreshold = durationThreshold;
    }
    /** What the browser gives this observer of `entries` of `type`. */
    kept(type: string, entries: readonly TimedEntry[]): TimedEntry[] {
      return type === "event" ? entries.filter((e) => e.duration >= this.eventThreshold) : [...entries];
    }
    takeRecords(): readonly TimedEntry[] {
      const taken = this.held;
      this.held = [];
      return taken;
    }
    disconnect(): void {
      this.disconnected = true;
    }
  }
  const of = (type: string) => observers.filter((o) => o.types.includes(type) && !o.disconnected);
  return {
    Observer: FakeObserver,
    observed: () => observers.flatMap((o) => o.types).toSorted(),
    allDisconnected: () => observers.length > 0 && observers.every((o) => o.disconnected),
    give: (type: string, entries: TimedEntry[]) => {
      for (const o of of(type)) {
        const kept = o.kept(type, entries);
        if (kept.length > 0) o.callback({ getEntries: () => kept });
      }
    },
    hold: (type: string, entries: TimedEntry[]) => {
      for (const o of of(type)) o.held.push(...o.kept(type, entries));
    },
  };
}

const paint = (startTime: number): TimedEntry => ({ startTime, duration: 0 });
const event = (duration: number, interactionId: number): TimedEntry => ({ startTime: 100, duration, interactionId });
const frame = (duration: number): TimedEntry => ({ startTime: 100, duration });

/** What the listeners heard, each in a list of its own. */
function heard() {
  const vitals = { largestPaint: [] as number[], interaction: [] as number[], longFrame: [] as number[] };
  return {
    vitals,
    listener: {
      largestPaint: (ms: number) => vitals.largestPaint.push(ms),
      interaction: (ms: number) => vitals.interaction.push(ms),
      longFrame: (ms: number) => vitals.longFrame.push(ms),
    },
  };
}

test("the largest paint is given once, the latest before the reader's first keydown", () => {
  const timeline = fakeTimeline();
  const target = new EventTarget();
  const { vitals, listener } = heard();
  const watch = watchVitals(listener, { Observer: timeline.Observer, target, hidden: false });
  expect(timeline.observed()).toEqual(allTypes.toSorted());

  timeline.give("largest-contentful-paint", [paint(1200)]);
  timeline.give("largest-contentful-paint", [paint(2500)]);
  expect(vitals.largestPaint).toEqual([]);

  target.dispatchEvent(new Event("keydown"));
  expect(vitals.largestPaint).toEqual([2500]);

  timeline.give("largest-contentful-paint", [paint(4000)]);
  target.dispatchEvent(new Event("pointerdown"));
  watch.settle();
  expect(vitals.largestPaint).toEqual([2500]);
  watch.stop();
});

test("a pointerdown ends the largest paint as a keydown does", () => {
  const timeline = fakeTimeline();
  const target = new EventTarget();
  const { vitals, listener } = heard();
  const watch = watchVitals(listener, { Observer: timeline.Observer, target, hidden: false });
  timeline.give("largest-contentful-paint", [paint(900)]);
  target.dispatchEvent(new Event("pointerdown"));
  expect(vitals.largestPaint).toEqual([900]);
  watch.stop();
});

test("without an input the page's settling gives the largest paint, with what the observer still held", () => {
  const timeline = fakeTimeline();
  const { vitals, listener } = heard();
  const watch = watchVitals(listener, { Observer: timeline.Observer, target: new EventTarget(), hidden: false });
  timeline.give("largest-contentful-paint", [paint(1800)]);
  timeline.hold("largest-contentful-paint", [paint(2100)]);
  watch.settle();
  expect(vitals.largestPaint).toEqual([2100]);
  watch.settle();
  expect(vitals.largestPaint).toEqual([2100]);
  watch.stop();
});

test("a page hidden as it loaded gives no largest paint", () => {
  const timeline = fakeTimeline();
  const target = new EventTarget();
  const { vitals, listener } = heard();
  const watch = watchVitals(listener, { Observer: timeline.Observer, target, hidden: true });
  timeline.give("largest-contentful-paint", [paint(1800)]);
  target.dispatchEvent(new Event("keydown"));
  watch.settle();
  expect(vitals.largestPaint).toEqual([]);
  watch.stop();
});

test("an interaction is heard over 40 ms, and an event of no interaction never", () => {
  const timeline = fakeTimeline();
  const { vitals, listener } = heard();
  const watch = watchVitals(listener, { Observer: timeline.Observer, target: new EventTarget(), hidden: false });
  timeline.give("event", [event(16, 3), event(300, 0), event(320, 4)]);
  timeline.give("event", [event(48, 5)]);
  expect(vitals.interaction).toEqual([320, 48]);
  watch.stop();
});

test("an interaction and a long frame the observer still held are heard as the page settles", () => {
  const timeline = fakeTimeline();
  const { vitals, listener } = heard();
  const watch = watchVitals(listener, { Observer: timeline.Observer, target: new EventTarget(), hidden: false });
  timeline.hold("event", [event(200, 9)]);
  timeline.hold("long-animation-frame", [frame(90)]);
  watch.settle();
  expect(vitals.interaction).toEqual([200]);
  expect(vitals.longFrame).toEqual([90]);
  watch.stop();
});

test("a long animation frame is heard by its duration", () => {
  const timeline = fakeTimeline();
  const { vitals, listener } = heard();
  const watch = watchVitals(listener, { Observer: timeline.Observer, target: new EventTarget(), hidden: false });
  timeline.give("long-animation-frame", [frame(120), frame(64)]);
  expect(vitals.longFrame).toEqual([120, 64]);
  watch.stop();
});

test("stop lets every observer go and the first input no longer ends the paint", () => {
  const timeline = fakeTimeline();
  const target = new EventTarget();
  const { vitals, listener } = heard();
  const watch = watchVitals(listener, { Observer: timeline.Observer, target, hidden: false });
  timeline.give("largest-contentful-paint", [paint(1000)]);
  watch.stop();
  expect(timeline.allDisconnected()).toBe(true);
  target.dispatchEvent(new Event("keydown"));
  expect(vitals.largestPaint).toEqual([]);
});

test("a browser without long animation frames is watched for the rest", () => {
  const timeline = fakeTimeline(["largest-contentful-paint", "event"]);
  const { vitals, listener } = heard();
  const watch = watchVitals(listener, { Observer: timeline.Observer, target: new EventTarget(), hidden: false });
  expect(timeline.observed()).not.toContain("long-animation-frame");
  timeline.give("event", [event(200, 1)]);
  expect(vitals.interaction).toEqual([200]);
  watch.stop();
});

test("a browser without a performance observer is watched for nothing, and throws nothing", () => {
  const { vitals, listener } = heard();
  const watch = watchVitals(listener, { Observer: undefined, target: new EventTarget(), hidden: false });
  watch.settle();
  watch.stop();
  expect(vitals).toEqual({ largestPaint: [], interaction: [], longFrame: [] });
});

test("the page's hiding is heard each time it hides, never as it shows, and not after the stop", () => {
  const target = new EventTarget();
  let hidden = false;
  let heardHidden = 0;
  const stop = watchPageHidden(
    () => (heardHidden += 1),
    target,
    () => hidden,
  );
  target.dispatchEvent(new Event("visibilitychange"));
  expect(heardHidden).toBe(0);
  hidden = true;
  target.dispatchEvent(new Event("visibilitychange"));
  expect(heardHidden).toBe(1);
  hidden = false;
  target.dispatchEvent(new Event("visibilitychange"));
  hidden = true;
  target.dispatchEvent(new Event("visibilitychange"));
  expect(heardHidden).toBe(2);
  stop();
  target.dispatchEvent(new Event("visibilitychange"));
  expect(heardHidden).toBe(2);
});

/** A browser's beacon that answers `accepts`, and what it was given; `restore` takes it away. */
function installBeacon(accepts: boolean) {
  const given: { url: string; data: unknown }[] = [];
  Object.defineProperty(navigator, "sendBeacon", {
    configurable: true,
    value: (url: string, data: unknown) => {
      given.push({ url, data });
      return accepts;
    },
  });
  return given;
}

afterEach(() => {
  Reflect.deleteProperty(navigator, "sendBeacon");
});

const url = "https://my.example/api/v1/client_measurements";
const body = JSON.stringify({ measurements: [{ metric: "lcp", name: "/_app/", value: 2500 }] });

/** The type and the text of what a beacon or a fetch sent. */
async function sent(data: unknown): Promise<{ type: string; text: string }> {
  if (data instanceof Blob) return { type: data.type, text: await data.text() };
  return { type: typeof data, text: String(data) };
}

test("a report leaves as a beacon of JSON, and no fetch follows a beacon the browser took", async () => {
  const given = installBeacon(true);
  const fetched = spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 204 }));
  try {
    sendReport(url, body);
    expect(given.map((g) => g.url)).toEqual([url]);
    const data = await sent(given[0]?.data);
    expect(data.type).toStartWith("application/json");
    expect(data.text).toBe(body);
    expect(fetched).not.toHaveBeenCalled();
  } finally {
    fetched.mockRestore();
  }
});

/** What the one fetch was asked: its method, address, keepalive, headers and body. */
async function fetchedRequest(fetched: { mock: { calls: Parameters<typeof fetch>[] } }) {
  expect(fetched.mock.calls).toHaveLength(1);
  const [input, init = {}] = fetched.mock.calls[0] ?? [""];
  const request = input instanceof Request ? input : null;
  const headers = new Headers(init.headers ?? request?.headers);
  return {
    method: init.method ?? request?.method,
    url: typeof input === "string" ? input : input instanceof URL ? input.href : input.url,
    keepalive: init.keepalive ?? request?.keepalive,
    type: headers.get("content-type") ?? (init.body instanceof Blob ? init.body.type : ""),
    authorization: headers.get("authorization"),
    body:
      init.body instanceof Blob
        ? await init.body.text()
        : typeof init.body === "string"
          ? init.body
          : await request?.text(),
  };
}

test("a beacon the browser refuses leaves as a POST of JSON that outlives the page", async () => {
  installBeacon(false);
  const fetched = spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 204 }));
  try {
    sendReport(url, body);
    const request = await fetchedRequest(fetched);
    expect([request.method, request.url, request.keepalive]).toEqual(["POST", url, true]);
    expect(request.type).toStartWith("application/json");
    expect(request.authorization).toBeNull();
    expect(request.body).toBe(body);
  } finally {
    fetched.mockRestore();
  }
});

test("a browser with no beacon sends the report with a fetch, and a failed fetch throws nothing", async () => {
  const fetched = spyOn(globalThis, "fetch").mockRejectedValue(new TypeError("offline"));
  try {
    sendReport(url, body);
    const request = await fetchedRequest(fetched);
    expect([request.method, request.keepalive]).toEqual(["POST", true]);
    expect(request.body).toBe(body);
    await Promise.resolve();
  } finally {
    fetched.mockRestore();
  }
});
