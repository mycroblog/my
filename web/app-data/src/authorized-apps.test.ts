// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { AppData } from "./app-data";
import { authorizedAppsQuery, revokeAuthorizedApp, useAuthorizedApps, useRevokeAuthorizedApp } from "./authorized-apps";
import { browserSessionsQuery } from "./browser-sessions";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { marked } from "./test/settings";

/** What a screen shown again shows: the cache while it holds the answer and it is not marked, else a read. */
const shown = (data: AppData) => data.queryClient.query({ ...authorizedAppsQuery(data), staleTime: Infinity });

async function read() {
  const given = fakeData();
  const { items: apps } = await given.data.queryClient.query(authorizedAppsQuery(given.data));
  await given.data.queryClient.query(browserSessionsQuery(given.data));
  return { ...given, apps };
}

describe("useAuthorizedApps", () => {
  test("is undefined until the list is read, then the apps, the server's own among them", async () => {
    const { data } = fakeData();
    expect(probe(data, useAuthorizedApps)).toBeUndefined();
    await data.queryClient.query(authorizedAppsQuery(data));
    const listed = probe(data, useAuthorizedApps);
    expect([listed?.length, listed?.filter((a) => a.first_party).length]).toEqual([4, 1]);
  });
});

describe("revokeAuthorizedApp", () => {
  test("revokes it, a screen shown again lists the others, and the browsers are not marked", async () => {
    const { data, apps } = await read();
    const gone = apps.find((a) => !a.first_party);
    await revokeAuthorizedApp(data, gone?.id ?? "");
    expect((await shown(data)).items.map((a) => a.id)).toEqual(apps.filter((a) => a !== gone).map((a) => a.id));
    expect(marked(data, browserSessionsQuery(data).queryKey)).toBe(false);
  });

  test("marks the browsers for a new read when it is the server's own app, which signs them all out", async () => {
    const { data, apps } = await read();
    await revokeAuthorizedApp(data, apps.find((a) => a.first_party)?.id ?? "");
    expect(marked(data, browserSessionsQuery(data).queryKey)).toBe(true);
    expect((await shown(data)).items.some((a) => a.first_party)).toBe(false);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data, apps } = await read();
    await probe(data, useRevokeAuthorizedApp)(apps.find((a) => !a.first_party)?.id ?? "");
    expect((await shown(data)).items.length).toBe(3);
  });
});
