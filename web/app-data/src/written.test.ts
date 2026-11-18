// SPDX-License-Identifier: AGPL-3.0-only

// What a write of the account settings leaves in the list its screen shows (#3205, #3206), pinned through the writes:
// a read of the list that was on its way when the server answered the write is not applied over the write's result;
// where the cache holds no list the write puts none, and the list is read anew; and a removal the server answers with
// 404 is done, since what it was to remove is gone: the write resolves, the list no longer holds the item, and the
// list is marked for a new read. Any other failure rejects and leaves the list as it was. What each write does
// besides is its own file's.

import { describe, expect, test } from "bun:test";
import { fakeOrigin, type FakeApi } from "@my/e2e/fake";
import type { QueryKey } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { authorizedAppsQuery, revokeAuthorizedApp } from "./authorized-apps";
import { browserSessionsQuery, revokeBrowserSession } from "./browser-sessions";
import { linkedIdentitiesQuery, unlinkIdentity } from "./linked-identities";
import { createOAuthApp, deleteOAuthApp, oAuthAppsQuery } from "./oauth-apps";
import { deletePasskey, passkeysQuery } from "./passkeys";
import { fakeData } from "./test/data";
import { marked, refusal, settled } from "./test/settings";

/** A list as the server answers it, in what these tests read of it. */
type Items = { readonly items: readonly { readonly id: string }[] };

/** A list of the account settings: the operation that reads it, its place in the cache, and a read of it. */
type Listed = {
  route: "GET /sessions" | "GET /passkeys" | "GET /linked_identities" | "GET /authorized_apps" | "GET /oauth_apps";
  key: (data: AppData) => QueryKey;
  /**
   * The list read as a screen reads it: the cache's while it is no older than `staleTime` and not marked for a new
   * read, else the server's.
   */
  at: (data: AppData, staleTime: number) => Promise<Items>;
};

const browsers: Listed = {
  route: "GET /sessions",
  key: (data) => browserSessionsQuery(data).queryKey,
  at: (data, staleTime) => data.queryClient.query({ ...browserSessionsQuery(data), staleTime }),
};
const passkeys: Listed = {
  route: "GET /passkeys",
  key: (data) => passkeysQuery(data).queryKey,
  at: (data, staleTime) => data.queryClient.query({ ...passkeysQuery(data), staleTime }),
};
const links: Listed = {
  route: "GET /linked_identities",
  key: (data) => linkedIdentitiesQuery(data).queryKey,
  at: (data, staleTime) => data.queryClient.query({ ...linkedIdentitiesQuery(data), staleTime }),
};
const authorized: Listed = {
  route: "GET /authorized_apps",
  key: (data) => authorizedAppsQuery(data).queryKey,
  at: (data, staleTime) => data.queryClient.query({ ...authorizedAppsQuery(data), staleTime }),
};
const own: Listed = {
  route: "GET /oauth_apps",
  key: (data) => oAuthAppsQuery(data).queryKey,
  at: (data, staleTime) => data.queryClient.query({ ...oAuthAppsQuery(data), staleTime }),
};

/** The path of `list` under the client API. */
const pathOf = (list: Listed) => list.route.replace("GET ", "");

/** The IDs of `list`'s items, in an order of their own: where a write puts a new item is its own file's. */
const ids = ({ items }: Items) => items.map((item) => item.id).sort();

/** The IDs of the items the cache holds of `list`; undefined where it holds no list. */
function held(data: AppData, list: Listed): string[] | undefined {
  const cached = data.queryClient.getQueryData<Items>(list.key(data));
  return cached === undefined ? undefined : ids(cached);
}

/** The IDs of the items the server holds of `list`, asked past the app's data. */
async function onServer(fake: FakeApi, list: Listed): Promise<string[]> {
  const answer = await fake.serve(new Request(`${fakeOrigin}/api/v1${pathOf(list)}`));
  return ids((await answer.json()) as Items);
}

/** How many times the app's data read `list`. */
const reads = (requests: readonly string[], list: Listed) =>
  requests.filter((request) => request === list.route).length;

/** A write and the list it changes: given the list's IDs before it, it resolves with the IDs after it. */
type Write = readonly [
  what: string,
  list: Listed,
  write: (data: AppData, before: readonly string[]) => Promise<string[]>,
];

// One removal and one addition stand for the eleven writes that leave their result in a list this way.
const writes: readonly Write[] = [
  [
    "a removal (deletePasskey)",
    passkeys,
    async (data, before) => {
      const [gone, ...kept] = before;
      await deletePasskey(data, gone ?? "");
      return kept;
    },
  ],
  [
    "an addition (createOAuthApp)",
    own,
    async (data, before) => {
      const { app } = await createOAuthApp(data, {
        name: "Reading list",
        redirect_uris: ["https://reading.example/callback"],
        scope: "read",
        client_type: "public",
      });
      return [...before, app.id].sort();
    },
  ],
];

describe("what a write leaves in the list a screen shows", () => {
  for (const [what, list, write] of writes) {
    test(`${what}: a read of the list that was on its way when the server answered the write is not applied over its result`, async () => {
      const { data, fake } = fakeData();
      const before = ids(await list.at(data, 0));
      // A read the server answered before the write, whose answer arrives after the write's.
      const older = fake.script(list.route, { hold: "response", times: 1 });
      const reading = list.at(data, 0).catch(() => undefined);
      await older.served();

      const after = await write(data, before);

      expect(after).not.toEqual(before);
      expect(held(data, list)).toEqual(after);
      older.release();
      await reading;
      await settled(data);
      expect(held(data, list)).toEqual(after);
      expect(await onServer(fake, list)).toEqual(after);
    });

    test(`${what}: where the cache holds no list it puts none, and the screen that shows the list reads it anew`, async () => {
      const { data, fake, requests } = fakeData();

      const after = await write(data, await onServer(fake, list));

      expect(held(data, list)).toBeUndefined();
      expect(reads(requests, list)).toBe(0);
      expect(ids(await list.at(data, Infinity))).toEqual(after);
      expect(reads(requests, list)).toBe(1);
    });
  }
});

/** A removal: the operation that removes, the list it removes from, the item of the fake's list it removes. */
type Removal = readonly [
  what: string,
  route:
    | "DELETE /sessions/{id}"
    | "DELETE /passkeys/{id}"
    | "DELETE /linked_identities/{id}"
    | "DELETE /authorized_apps/{id}"
    | "DELETE /oauth_apps/{id}",
  list: Listed,
  remove: (data: AppData, id: string) => Promise<unknown>,
  at: number,
];

const removals: readonly Removal[] = [
  // The fake's first browser is the one that asks, and its first authorized app the server's own web client.
  ["a browser", "DELETE /sessions/{id}", browsers, revokeBrowserSession, 1],
  ["a passkey", "DELETE /passkeys/{id}", passkeys, deletePasskey, 0],
  ["a linked account", "DELETE /linked_identities/{id}", links, unlinkIdentity, 0],
  ["an app the account authorized", "DELETE /authorized_apps/{id}", authorized, revokeAuthorizedApp, 1],
  ["an app of the reader's", "DELETE /oauth_apps/{id}", own, deleteOAuthApp, 0],
];

/** The data after `list` was read, and the item at `at` of it as the server lists it. */
async function listed(list: Listed, at: number) {
  const given = fakeData();
  const { items } = await list.at(given.data, 0);
  const id = items[at]?.id;
  if (id === undefined) throw new Error(`test: the fake's list has no item at ${String(at)}`);
  return { ...given, id, before: ids({ items }) };
}

describe("a removal the server answers with 404, since what it was to remove is gone", () => {
  for (const [what, , list, remove, at] of removals)
    test(`${what}: the write resolves, the list no longer holds it at once, and the screen that shows the list reads it once`, async () => {
      const { data, fake, requests, id, before } = await listed(list, at);
      // Another browser of the reader's removed it first.
      const first = await fake.serve(new Request(`${fakeOrigin}/api/v1${pathOf(list)}/${id}`, { method: "DELETE" }));
      expect(first.status).toBe(204);

      await remove(data, id);

      // Nothing was read since: the list holds what the write left in it.
      expect(requests).toEqual([list.route, `DELETE ${pathOf(list)}/${id}`]);
      expect(held(data, list)).toEqual(before.filter((one) => one !== id));
      expect(marked(data, list.key(data))).toBe(true);
      expect(ids(await list.at(data, Infinity))).toEqual(before.filter((one) => one !== id));
      expect(reads(requests, list)).toBe(2);
    });
});

describe("a removal the server fails otherwise", () => {
  for (const [what, route, list, remove, at] of removals)
    test(`${what}: the write rejects with the server's problem, and the list holds it still`, async () => {
      const { data, fake, id, before } = await listed(list, at);
      fake.script(route, { fail: { type: "about:blank", title: "The server could not do this", status: 500 } });

      const refused = await refusal(remove(data, id));

      expect([refused.problem.status, refused.problem.title]).toEqual([500, "The server could not do this"]);
      expect(held(data, list)).toEqual(before);
      expect(await onServer(fake, list)).toEqual(before);
    });
});
