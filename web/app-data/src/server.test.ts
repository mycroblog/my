// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { watchServer, webClientEpoch, type ServerState, type ServerWatch } from "./server";
import { json } from "./test/fake";

const server = "https://example.com";
const health = `${server}/api/v1/instance/health`;
const home = () => new Request(`${server}/api/v1/timelines/home`);

type Mode = "up" | "maintenance" | "offline" | "outage";

/**
 * A server in `mode`, which the test changes: up, it answers every operation with an empty page and its health `ok`;
 * under maintenance, both with the maintenance's 503; offline, the device reaches nothing; in an outage, it answers 503
 * and its health `unavailable`. `epoch` is the X-Web-Client-Epoch it sends. It counts the health checks it got.
 */
function fakeServer() {
  const s = { mode: "up" as Mode, epoch: String(webClientEpoch), checks: 0 };
  const maintenance = {
    type: "/api/v1/problems/maintenance",
    title: "Under maintenance",
    status: 503,
    message: "Moving the database to a larger disk.",
    until: "2026-11-15T04:00:00Z",
    status_page_url: "https://status.example.com",
  };
  const fetch = (request: Request): Promise<Response> => {
    const isHealth = request.url === health;
    if (isHealth) s.checks += 1;
    let answer: Response;
    switch (s.mode) {
      case "offline":
        return Promise.reject(new TypeError("Failed to fetch"));
      case "maintenance":
        answer = isHealth ? json(503, { status: "maintenance" }) : json(503, maintenance);
        answer.headers.set("Retry-After", "3600");
        break;
      case "outage":
        answer = isHealth ? json(503, { status: "unavailable" }) : json(503, { status: 503, title: "Unavailable" });
        break;
      case "up":
        answer = isHealth ? json(200, { status: "ok" }) : json(200, { items: [] });
    }
    answer.headers.set("X-Web-Client-Epoch", s.epoch);
    return Promise.resolve(answer);
  };
  return { s, fetch, maintenance };
}

/** A watch of `fetch` that rechecks the health after `recheckMs` and ignores the test machine's network. */
const watch = (fetch: (r: Request) => Promise<Response>, recheckMs = 60_000) =>
  watchServer({ server, fetch, recheckMs, followDevice: false });

/** Resolves once the watch's state meets `wanted`, now or at one of its changes. */
function until(w: ServerWatch, wanted: (s: ServerState) => boolean): Promise<ServerState> {
  return new Promise((resolve) => {
    if (wanted(w.state())) {
      resolve(w.state());
      return;
    }
    const stop = w.subscribe(() => {
      if (!wanted(w.state())) return;
      stop();
      resolve(w.state());
    });
  });
}

/** Sends a request through the watch, whatever its outcome. */
const send = (w: ServerWatch, request = home()) => w.fetch(request).catch(() => undefined);

test("a server that answers leaves the state healthy, and the answer reaches the client as the server sent it", async () => {
  const { fetch } = fakeServer();
  const w = watch(fetch);
  const answer = await w.fetch(home());
  expect(answer.status).toBe(200);
  expect(await answer.json()).toEqual({ items: [] });
  expect(w.state()).toEqual({ connection: "online", maintenance: null, update: "none" });
  // The same object until the state changes, as useSyncExternalStore needs.
  expect(w.state()).toBe(w.state());
  w.stop();
});

test("the maintenance problem shows the maintenance with the operator's notice, its end and the status page", async () => {
  const { s, fetch, maintenance } = fakeServer();
  const w = watch(fetch);
  let changes = 0;
  w.subscribe(() => {
    changes += 1;
  });
  s.mode = "maintenance";
  await send(w);
  const state = await until(w, (x) => x.maintenance !== null);
  expect(state.maintenance).toEqual({
    message: maintenance.message,
    until: maintenance.until,
    statusPageUrl: maintenance.status_page_url,
  });
  expect(changes).toBeGreaterThan(0);
  s.mode = "up";
  await w.check();
  expect(w.state().maintenance).toBeNull();
  expect(w.state().connection).toBe("online");
  w.stop();
});

test("a request that reaches nothing while the health reaches nothing either is the reader offline", async () => {
  const { s, fetch } = fakeServer();
  const w = watch(fetch);
  s.mode = "offline";
  await send(w);
  expect((await until(w, (x) => x.connection !== "online")).connection).toBe("offline");
  w.stop();
});

test("a server that answers 503 and says its health is unavailable is an outage, not the reader offline", async () => {
  const { s, fetch } = fakeServer();
  const w = watch(fetch);
  s.mode = "outage";
  await send(w);
  expect((await until(w, (x) => x.connection !== "online")).connection).toBe("outage");
  expect(w.state().maintenance).toBeNull();
  w.stop();
});

test("while the server does not answer the watch asks its health again, and is online once it answers", async () => {
  const { s, fetch } = fakeServer();
  const w = watch(fetch, 5);
  s.mode = "outage";
  await send(w);
  await until(w, (x) => x.connection === "outage");
  const checks = s.checks;
  s.mode = "up";
  // No request of the app: the watch's own check finds the server back.
  expect((await until(w, (x) => x.connection === "online")).connection).toBe("online");
  expect(s.checks).toBeGreaterThan(checks);
  w.stop();
});

test("asked by hand, the health settles the connection: offline, an outage, then online", async () => {
  const { s, fetch } = fakeServer();
  const w = watch(fetch);
  s.mode = "offline";
  await w.check();
  expect(w.state().connection).toBe("offline");
  s.mode = "outage";
  await w.check();
  expect(w.state().connection).toBe("outage");
  s.mode = "up";
  await w.check();
  expect(w.state().connection).toBe("online");
  w.stop();
});

test("an epoch of the server above the app's requires an update; the same epoch requires none", async () => {
  const { s, fetch } = fakeServer();
  const w = watch(fetch);
  await send(w);
  expect(w.state().update).toBe("none");
  s.epoch = String(webClientEpoch + 1);
  await send(w);
  expect(w.state().update).toBe("required");
  // A required update stays: the reader cannot close it.
  w.dismissUpdate();
  expect(w.state().update).toBe("required");
  w.stop();
});

test("a build whose epoch the server's equals needs no update", async () => {
  const { s, fetch } = fakeServer();
  s.epoch = "7";
  const w = watchServer({ server, fetch, epoch: 7, followDevice: false });
  await send(w);
  expect(w.state().update).toBe("none");
  w.stop();
});

test("a newer build offered is ready, and closing its toast puts it away until the app loads again", () => {
  const { fetch } = fakeServer();
  const w = watch(fetch);
  w.offerUpdate();
  expect(w.state().update).toBe("ready");
  w.dismissUpdate();
  expect(w.state().update).toBe("none");
  w.stop();
});
