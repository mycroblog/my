// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { createFakeApi, fakeOrigin, fakeStatusPage, type FakeApi } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import { createAppData, type AppData } from "./app-data";
import { instanceQuery, useStatusPageUrl } from "./instance";
import { watchServer } from "./server";
import { probe } from "./test/probe";

const fromInstance = "https://status.example.com";
/** The status page the fake's maintenance names. */
const fromMaintenance = fakeStatusPage;
const kept = "https://kept-status.example.com";

/**
 * A device whose storage holds `url` under every key that names `holder`, as if an earlier load of the app had kept
 * the status page of that server; Bun has no storage of its own.
 */
function deviceKeeping(holder: string, url: string): Storage {
  return {
    length: 0,
    clear: () => undefined,
    getItem: (key) => (key.includes(holder) ? url : null),
    key: () => null,
    removeItem: () => undefined,
    setItem: () => undefined,
  };
}

afterEach(() => {
  Reflect.deleteProperty(globalThis, "localStorage");
});

/**
 * A server whose instance names `statusPage` in place of the fake's own, up at first, then as the test sets it: under
 * maintenance (whose problem names the fake's status page) or not answering.
 */
function fakeServer(statusPage: string | undefined) {
  const up = createFakeApi();
  up.script("GET /instance", {
    change: ({ status_page_url: _, ...instance }) => ({
      ...instance,
      ...(statusPage === undefined ? {} : { status_page_url: statusPage }),
    }),
  });
  const maintenance = createFakeApi({ maintenance: true });
  const down = createFakeApi({ down: "offline" });
  // An offline fake still answers the instance it was asked first, as the shell's read before the network went.
  down.script("GET /instance", { fail: "network" });
  const s: { now: FakeApi } = { now: up };
  return { s, maintenance, down, fetch: (request: Request) => s.now.serve(request) };
}

/** The app's data on a load of the app: its own cache, through a watch of the server. */
function load(fetch: (request: Request) => Promise<Response>): AppData {
  const watch = watchServer({ server: fakeOrigin, fetch, followDevice: false });
  return createAppData(
    createClient({ server: fakeOrigin, fetch: watch.fetch, retry: { disabled: true } }),
    undefined,
    watch,
  );
}

/** The app's data after it asked the instance, whether or not the server answered. */
async function loaded(fetch: (request: Request) => Promise<Response>): Promise<AppData> {
  const data = load(fetch);
  await data.queryClient.query(instanceQuery(data)).catch(() => undefined);
  return data;
}

/** The status page the shell links to now. */
const statusPage = (data: AppData) => probe(data, useStatusPageUrl);

test("the status page is the maintenance's, else the instance's, else the one the device kept from an earlier load", async () => {
  globalThis.localStorage = deviceKeeping(fakeOrigin, kept);
  const { s, maintenance, down, fetch } = fakeServer(fromInstance);
  const data = await loaded(fetch);
  expect(statusPage(data)).toBe(fromInstance);

  s.now = maintenance;
  await data.api.GET("/timelines/home").catch(() => undefined);
  expect(statusPage(data)).toBe(fromMaintenance);

  // The app loads again while the server does not answer: no instance, and the device's address.
  s.now = down;
  expect(statusPage(await loaded(fetch))).toBe(kept);
});

test("an instance without a status page has none, and a device that kept another server's or none links to none", async () => {
  globalThis.localStorage = deviceKeeping("https://other.example", "https://status.other.example");
  const { s, down, fetch } = fakeServer(undefined);
  expect(statusPage(await loaded(fetch))).toBeUndefined();
  s.now = down;
  expect(statusPage(await loaded(fetch))).toBeUndefined();
  Reflect.deleteProperty(globalThis, "localStorage");
  expect(statusPage(await loaded(fetch))).toBeUndefined();
});
