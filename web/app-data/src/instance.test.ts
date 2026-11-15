// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { createClient } from "@mycroblog/mykit";
import { createAppData, type AppData } from "./app-data";
import { instanceQuery, useStatusPageUrl } from "./instance";
import { watchServer } from "./server";
import { json } from "./test/fake";
import { probe } from "./test/probe";

const server = "https://example.com";
const fromInstance = "https://status.example.com";
const fromMaintenance = "https://maintenance-status.example.com";
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
 * A server whose instance names `statusPage`, up or under maintenance (whose problem names its own status page) or
 * down, as the test sets it.
 */
function fakeServer(statusPage: string | undefined) {
  const s = { mode: "up" as "up" | "maintenance" | "down" };
  const fetch = (request: Request): Promise<Response> => {
    const path = new URL(request.url).pathname;
    if (s.mode === "down") return Promise.reject(new TypeError("Failed to fetch"));
    if (s.mode === "maintenance")
      return Promise.resolve(
        json(503, {
          type: "/api/v1/problems/maintenance",
          title: "Under maintenance",
          status: 503,
          status_page_url: fromMaintenance,
        }),
      );
    if (path === "/api/v1/instance")
      return Promise.resolve(
        json(200, { domain: "example.com", ...(statusPage === undefined ? {} : { status_page_url: statusPage }) }),
      );
    if (path === "/api/v1/instance/health") return Promise.resolve(json(200, { status: "ok" }));
    return Promise.resolve(json(200, { items: [] }));
  };
  return { s, fetch };
}

/** The app's data on a load of the app: its own cache, through a watch of the server. */
function load(fetch: (request: Request) => Promise<Response>): AppData {
  const watch = watchServer({ server, fetch, followDevice: false });
  return createAppData(createClient({ server, fetch: watch.fetch, retry: { disabled: true } }), undefined, watch);
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
  globalThis.localStorage = deviceKeeping(server, kept);
  const { s, fetch } = fakeServer(fromInstance);
  const data = await loaded(fetch);
  expect(statusPage(data)).toBe(fromInstance);

  s.mode = "maintenance";
  await data.api.GET("/timelines/home").catch(() => undefined);
  expect(statusPage(data)).toBe(fromMaintenance);

  // The app loads again while the server does not answer: no instance, and the device's address.
  s.mode = "down";
  expect(statusPage(await loaded(fetch))).toBe(kept);
});

test("an instance without a status page has none, and a device that kept another server's or none links to none", async () => {
  globalThis.localStorage = deviceKeeping("https://other.example", "https://status.other.example");
  const { s, fetch } = fakeServer(undefined);
  expect(statusPage(await loaded(fetch))).toBeUndefined();
  s.mode = "down";
  expect(statusPage(await loaded(fetch))).toBeUndefined();
  Reflect.deleteProperty(globalThis, "localStorage");
  expect(statusPage(await loaded(fetch))).toBeUndefined();
});
