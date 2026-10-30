// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { ProblemError } from "./problem.js";
import {
  HeartbeatTimeoutError,
  NotEventStreamError,
  parseStreamEvent,
  runStream,
  streamDelay,
  type StreamEvent,
  type StreamOptions,
} from "./stream.js";
import { fakeFetch, fakeSleep, problem, server, type Handler } from "./test/fake.js";
import { staticToken, type TokenSource } from "./token.js";

const encoder = new TextEncoder();

/**
 * An event stream of the chunks, each after `gap` ms. With `hold` it stays open after them until the client cancels
 * it, as a server that has nothing more to send.
 */
function sse(chunks: string[], { hold = false, gap = 0 }: { hold?: boolean; gap?: number } = {}): Response {
  const body = new ReadableStream<Uint8Array>({
    async start(controller) {
      for (const chunk of chunks) {
        if (gap > 0) await new Promise((resolve) => setTimeout(resolve, gap));
        controller.enqueue(encoder.encode(chunk));
      }
      if (!hold) controller.close();
    },
  });
  return new Response(body, { headers: { "Content-Type": "text/event-stream" } });
}

/** A fetch that answers the n-th request (from 0) with the n-th handler, and the later ones with the last. */
function connections(...handlers: Handler[]) {
  let n = 0;
  return fakeFetch((r) => {
    const handler = handlers[Math.min(n++, handlers.length - 1)];
    if (handler === undefined) throw new Error("no handler");
    return handler(r);
  });
}

/** Runs the stream with options until its open number `stopAt` (from 0), recording what it reports. */
async function run(
  options: Omit<StreamOptions, "server" | "signal" | "onEvent" | "onOpen" | "onError">,
  stopAt: number,
) {
  const controller = new AbortController();
  const opens: boolean[] = [];
  const events: StreamEvent[] = [];
  const errors: unknown[] = [];
  const done = runStream({
    server,
    signal: controller.signal,
    onOpen: (reconnected) => {
      opens.push(reconnected);
      if (opens.length > stopAt) controller.abort();
    },
    onEvent: (e) => events.push(e),
    onError: (e) => errors.push(e),
    ...options,
  });
  let outcome: unknown = null;
  await done.catch((error: unknown) => {
    outcome = error;
  });
  return { opens, events, errors, outcome };
}

describe("runStream", () => {
  test("delivers typed events, passes an unknown one on, and tells a reconnection", async () => {
    const f = connections(
      () =>
        sse([
          ": connected\n\n",
          'event: update\ndata: {"id":"s1"}\n\n',
          'event: list_update\ndata: {"list_id":"l1","status":{"id":"s1"}}\n\n',
          'event: notification\ndata: {"id":"n1","type":"follow"}\n\n',
          ": heartbeat\n\n",
          'event: announcement\ndata: {"id":"a1"}\n\n',
          'event: announcement_removed\ndata: {"id":"a2"}\n\n',
          'event: poll\ndata: {"id":"p1"}\ndata: more\n\n',
          "event: update\ndata: not json\n\n",
        ]),
      () => sse([": connected\n\n"], { hold: true }),
    );
    const s = fakeSleep();
    const r = await run({ fetch: f.fetch, sleep: s.sleep, tokenSource: staticToken("a0"), acceptLanguage: "ja" }, 1);

    expect(r.outcome).toBeNull();
    expect(r.opens).toEqual([false, true]);
    expect(r.events.map((e) => e.type)).toEqual([
      "update",
      "list_update",
      "notification",
      "announcement",
      "announcement_removed",
      "unknown",
    ]);
    const [update, listUpdate, notification, announcement, removed, unknown] = r.events;
    if (update?.type === "update") expect(update.status.id).toBe("s1");
    if (listUpdate?.type === "list_update") {
      expect(listUpdate.listId).toBe("l1");
      expect(listUpdate.status.id).toBe("s1");
    }
    if (notification?.type === "notification") expect(notification.notification.id).toBe("n1");
    if (announcement?.type === "announcement") expect(announcement.announcement.id).toBe("a1");
    expect(removed).toEqual({ type: "announcement_removed", id: "a2" });
    expect(unknown).toEqual({ type: "unknown", event: "poll", data: '{"id":"p1"}\nmore' });
    // The update whose data is no JSON is skipped and reported, and the stream goes on.
    expect(r.errors).toHaveLength(1);
    // A stream that ended normally waits about a second before connecting again.
    expect(s.waits).toHaveLength(1);
    expect(s.waits[0]).toBeGreaterThanOrEqual(500);
    expect(s.waits[0]).toBeLessThanOrEqual(1000);
    for (const { request } of f.requests) {
      expect(request.url).toBe(`${server}/api/v1/streaming`);
      expect(request.headers.get("Authorization")).toBe("Bearer a0");
      expect(request.headers.get("Accept")).toBe("text/event-stream");
      expect(request.headers.get("Accept-Language")).toBe("ja");
      expect(request.headers.has("Last-Event-ID")).toBe(false);
    }
  });

  test("reads the stream of the local or the federated timeline", async () => {
    for (const timeline of ["local", "federated"] as const) {
      const f = connections(
        () => sse([": connected\n\n", 'event: update\ndata: {"id":"s1"}\n\n']),
        () => sse([": connected\n\n"], { hold: true }),
      );
      const r = await run({ fetch: f.fetch, sleep: fakeSleep().sleep, tokenSource: staticToken("a0"), timeline }, 1);
      expect(r.opens).toEqual([false, true]);
      expect(r.events.map((e) => e.type)).toEqual(["update"]);
      for (const { request } of f.requests) {
        expect(request.url).toBe(`${server}/api/v1/streaming/timelines/${timeline}`);
        expect(request.headers.get("Authorization")).toBe("Bearer a0");
      }
    }
  });

  test("drops a stream whose heartbeats stop and connects again", async () => {
    const f = connections(() => sse([": connected\n\n"], { hold: true }));
    const r = await run({ fetch: f.fetch, sleep: fakeSleep().sleep, heartbeatTimeout: 50 }, 1);
    expect(r.opens).toEqual([false, true]);
    expect(r.errors).toHaveLength(1);
    expect(r.errors[0]).toBeInstanceOf(HeartbeatTimeoutError);
  });

  test("keeps a stream whose heartbeats arrive in time", async () => {
    const f = connections(
      () =>
        sse(
          [
            ": connected\n\n",
            ": heartbeat\n\n",
            ": heartbeat\n\n",
            ": heartbeat\n\n",
            'event: announcement_removed\ndata: {"id":"a1"}\n\n',
          ],
          { gap: 20 },
        ),
      () => sse([": connected\n\n"], { hold: true }),
    );
    const r = await run({ fetch: f.fetch, sleep: fakeSleep().sleep, heartbeatTimeout: 300 }, 1);
    expect(r.errors).toEqual([]);
    expect(r.events).toEqual([{ type: "announcement_removed", id: "a1" }]);
  });

  test("waits for Retry-After after a 429 and a 503, and backs off without one", async () => {
    const f = connections(
      () => problem(429, "/api/v1/problems/too-many-streams", {}, { "Retry-After": "7" }),
      () => problem(503, "about:blank", {}, { "Retry-After": "5" }),
      () => problem(502, "about:blank"),
      () => sse([": connected\n\n"], { hold: true }),
    );
    const s = fakeSleep();
    const r = await run({ fetch: f.fetch, sleep: s.sleep }, 0);
    expect(r.opens).toEqual([false]);
    expect(s.waits.slice(0, 2)).toEqual([7000, 5000]);
    // The third failure in a row backs off from 8 s, with jitter over its upper half.
    expect(s.waits[2]).toBeGreaterThanOrEqual(4000);
    expect(s.waits[2]).toBeLessThanOrEqual(8000);
    expect(r.errors).toHaveLength(3);
    expect(r.errors[0]).toBeInstanceOf(ProblemError);
    expect((r.errors[0] as ProblemError).type).toBe("/api/v1/problems/too-many-streams");
  });

  test("refreshes the token once after a 401", async () => {
    let refreshes = 0;
    const tokens: TokenSource = {
      getAccessToken: () => Promise.resolve("a0"),
      refresh: () => {
        refreshes++;
        return Promise.resolve("a1");
      },
    };
    const f = connections((r) =>
      r.headers.get("Authorization") === "Bearer a1"
        ? sse([": connected\n\n"], { hold: true })
        : problem(401, "/api/v1/problems/invalid-token"),
    );
    const r = await run({ fetch: f.fetch, sleep: fakeSleep().sleep, tokenSource: tokens }, 0);
    expect(refreshes).toBe(1);
    expect(r.errors).toEqual([]);
    expect(r.opens).toEqual([false]);
  });

  test("rejects without connecting again when the server refuses the request for good", async () => {
    const f = connections(() => problem(403, "/api/v1/problems/insufficient-scope"));
    const s = fakeSleep();
    const r = await run({ fetch: f.fetch, sleep: s.sleep, tokenSource: staticToken("a0") }, 0);
    expect(r.outcome).toBeInstanceOf(ProblemError);
    expect((r.outcome as ProblemError).status).toBe(403);
    expect(s.waits).toEqual([]);
    expect(f.requests).toHaveLength(1);
  });

  test("connects again after an answer that is no event stream", async () => {
    const f = connections(
      () => new Response("<p>maintenance</p>", { headers: { "Content-Type": "text/html" } }),
      () => sse([": connected\n\n"], { hold: true }),
    );
    const r = await run({ fetch: f.fetch, sleep: fakeSleep().sleep }, 0);
    expect(r.errors).toHaveLength(1);
    expect(r.errors[0]).toBeInstanceOf(NotEventStreamError);
    expect(r.opens).toEqual([false]);
  });

  test("resolves when the signal aborts while it waits", async () => {
    const controller = new AbortController();
    const f = connections(() => problem(502, "about:blank"));
    await runStream({
      server,
      fetch: f.fetch,
      signal: controller.signal,
      onEvent: () => undefined,
      onError: () => {
        controller.abort();
      },
    });
    expect(controller.signal.aborted).toBe(true);
  });
});

describe("parseStreamEvent", () => {
  test("throws for a known event without an id, and passes an unknown one on unparsed", () => {
    expect(() => parseStreamEvent({ event: "announcement_removed", data: "{}" })).toThrow();
    expect(() => parseStreamEvent({ event: "list_update", data: '{"list_id":"l1"}' })).toThrow();
    expect(() => parseStreamEvent({ event: "list_update", data: '{"id":"s1"}' })).toThrow();
    expect(parseStreamEvent({ event: "message", data: "x" })).toEqual({ type: "unknown", event: "message", data: "x" });
  });
});

describe("streamDelay", () => {
  test("doubles with each failure, with jitter over its upper half, up to a minute", () => {
    for (const [failures, limit] of [
      [0, 1000],
      [1, 2000],
      [2, 4000],
    ] as const) {
      for (let i = 0; i < 20; i++) {
        const d = streamDelay(failures);
        expect(d).toBeGreaterThanOrEqual(limit / 2);
        expect(d).toBeLessThanOrEqual(limit);
      }
    }
    expect(streamDelay(30)).toBeLessThanOrEqual(60_000);
  });
});
