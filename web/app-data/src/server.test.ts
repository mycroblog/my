// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { createFakeApi, fakeOrigin, type FakeApi } from "@my/e2e/fake";
import { watchServer, type ServerState, type ServerWatch } from "./server";

const home = () => new Request(`${fakeOrigin}/api/v1/timelines/home`);

type Mode = "up" | "maintenance" | "offline" | "outage" | "newer";

/**
 * A server in `mode`, which the test changes, each mode a fake API in that state: up, it answers every operation and
 * its health `ok`; under maintenance, both with the maintenance's 503; offline, the device reaches nothing; in an
 * outage, it answers 503 and its health `unavailable`; newer, it is up and sends an X-Web-Client-Epoch above the
 * app's (999). `checks()` counts the health checks it got in a mode.
 */
function fakeServer() {
  const modes: Record<Mode, FakeApi> = {
    up: createFakeApi(),
    maintenance: createFakeApi({ maintenance: true }),
    offline: createFakeApi({ down: "offline" }),
    outage: createFakeApi({ down: "outage" }),
    newer: createFakeApi({ newerEpoch: true }),
  };
  const health = modes.up.script("GET /instance/health");
  const s = { mode: "up" as Mode };
  const fetch = (request: Request): Promise<Response> => modes[s.mode].serve(request);
  return { s, fetch, modes, checks: () => health.seen.length };
}

/** A watch of `fetch` that rechecks the health after `recheckMs` and ignores the test machine's network. */
const watch = (fetch: (r: Request) => Promise<Response>, recheckMs = 60_000) =>
  watchServer({ server: fakeOrigin, fetch, recheckMs, followDevice: false });

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
  const { fetch, modes } = fakeServer();
  const w = watch(fetch);
  const answer = await w.fetch(home());
  expect(answer.status).toBe(200);
  const sent: unknown = await (await modes.up.serve(home())).json();
  expect(await answer.json()).toEqual(sent);
  expect(w.state()).toEqual({ connection: "online", maintenance: null, update: "none" });
  // The same object until the state changes, as useSyncExternalStore needs.
  expect(w.state()).toBe(w.state());
  w.stop();
});

test("the maintenance problem shows the maintenance with the operator's notice, its end and the status page", async () => {
  const { s, fetch, modes } = fakeServer();
  const w = watch(fetch);
  let changes = 0;
  w.subscribe(() => {
    changes += 1;
  });
  s.mode = "maintenance";
  await send(w);
  const state = await until(w, (x) => x.maintenance !== null);
  const problem = (await (await modes.maintenance.serve(home())).json()) as Record<string, unknown>;
  expect(problem["type"]).toBe("/api/v1/problems/maintenance");
  expect(state.maintenance).toEqual({
    message: expect.stringContaining("Moving the database to a larger disk."),
    until: expect.stringMatching(/^\d{4}-\d{2}-\d{2}T/),
    statusPageUrl: "https://status.demo.test",
  });
  expect(state.maintenance as unknown).toEqual({
    message: problem["message"],
    until: problem["until"],
    statusPageUrl: problem["status_page_url"],
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
  const { s, fetch, checks } = fakeServer();
  const w = watch(fetch, 5);
  s.mode = "outage";
  await send(w);
  await until(w, (x) => x.connection === "outage");
  expect(checks()).toBe(0);
  s.mode = "up";
  // No request of the app: the watch's own check finds the server back.
  expect((await until(w, (x) => x.connection === "online")).connection).toBe("online");
  expect(checks()).toBeGreaterThan(0);
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
  s.mode = "newer";
  await send(w);
  expect(w.state().update).toBe("required");
  // A required update stays: the reader cannot close it.
  w.dismissUpdate();
  expect(w.state().update).toBe("required");
  w.stop();
});

test("a build whose epoch the server's equals needs no update", async () => {
  const { s, fetch } = fakeServer();
  s.mode = "newer";
  const w = watchServer({ server: fakeOrigin, fetch, epoch: 999, followDevice: false });
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
