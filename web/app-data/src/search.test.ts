// SPDX-License-Identifier: AGPL-3.0-only

// The search (FR-305–FR-307, FR-309): the words that name a remote account; what each kind asks the server; and the
// search as one, which is pending while a kind it asks for is, failed when one failed, and asks again only what failed;
// posts only where the instance offers full-text search.

import { describe, expect, test } from "bun:test";
import type { AppData } from "./app-data";
import { instanceQuery } from "./instance";
import {
  accountResultsQuery,
  postResultsQuery,
  remoteAccountQuery,
  remoteOf,
  tagResultsQuery,
  usePostSearchOffered,
  useSearch,
} from "./search";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

describe("remoteOf", () => {
  for (const [words, remote] of [
    ["@kiri@far.example", "kiri@far.example"],
    ["kiri@far.example", "kiri@far.example"],
    ["  @kiri@far.example  ", "kiri@far.example"],
    ["https://far.example/@kiri", "https://far.example/@kiri"],
    ["http://far.example/users/kiri", "http://far.example/users/kiri"],
  ] as const)
    test(`"${words}" names the remote account ${remote}`, () => {
      expect(remoteOf(words)).toBe(remote);
    });

  for (const words of ["sam", "@sam", "kiri@localhost", "kiri@far.example and more", "far.example", "", "#baking"])
    test(`"${words}" names no remote account`, () => {
      expect(remoteOf(words)).toBeUndefined();
    });
});

/**
 * A server whose instance offers full-text search or not (`offered`): for "bak" it finds the account of Bakery Kobo,
 * the hashtag #baking and sam's posts of it, and by a lookup the account kiri@far.example, which it has not met.
 */
function server(offered: boolean) {
  return fakeData({ search: { posts: offered } });
}

/** The query of each request of `path` among `requests`. */
const asked = (requests: readonly string[], path: string) =>
  requests
    .map((r) => new URL(r.slice("GET ".length), "https://demo.test"))
    .filter((u) => u.pathname === path)
    .map((u) => Object.fromEntries(u.searchParams));

/** Reads each query `q` asks for into the cache, as the screen's observers would, and lets a failure stay cached. */
async function searched(data: AppData, q: string, kinds: { posts?: boolean; remote?: string } = {}) {
  const settle = (p: Promise<unknown>) =>
    p.then(
      () => undefined,
      () => undefined,
    );
  await settle(data.queryClient.query(instanceQuery(data)));
  await Promise.all([
    settle(data.queryClient.query(accountResultsQuery(data, q))),
    settle(data.queryClient.query(tagResultsQuery(data, q))),
    ...(kinds.posts === true ? [settle(data.queryClient.infiniteQuery(postResultsQuery(data, q)))] : []),
    ...(kinds.remote === undefined ? [] : [settle(data.queryClient.query(remoteAccountQuery(data, kinds.remote)))]),
  ]);
}

describe("each kind's query", () => {
  test("accounts are searched by the words", async () => {
    const { data, requests } = server(false);
    await data.queryClient.query(accountResultsQuery(data, "bak"));
    expect(asked(requests, "/accounts/search").map((p) => p["q"])).toEqual(["bak"]);
  });

  test("hashtags are searched by the words", async () => {
    const { data, requests } = server(false);
    await data.queryClient.query(tagResultsQuery(data, "bak"));
    expect(asked(requests, "/tags/search").map((p) => p["q"])).toEqual(["bak"]);
  });

  test("a remote account is looked up on its server, with resolve", async () => {
    const { data, requests } = server(false);
    await data.queryClient.query(remoteAccountQuery(data, "kiri@far.example"));
    expect(asked(requests, "/accounts/lookup")).toEqual([{ acct: "kiri@far.example", resolve: "true" }]);
  });

  test("posts are searched by the words", async () => {
    const { data, requests } = server(true);
    await data.queryClient.infiniteQuery(postResultsQuery(data, "bak"));
    expect(asked(requests, "/statuses/search").map((p) => p["q"])).toEqual(["bak"]);
  });
});

describe("usePostSearchOffered", () => {
  test("undefined until the instance is read, then whether it offers full-text search", async () => {
    const off = server(false);
    expect(probe(off.data, usePostSearchOffered)).toBeUndefined();
    await off.data.queryClient.query(instanceQuery(off.data));
    expect(probe(off.data, usePostSearchOffered)).toBe(false);
    const on = server(true);
    await on.data.queryClient.query(instanceQuery(on.data));
    expect(probe(on.data, usePostSearchOffered)).toBe(true);
  });
});

describe("useSearch", () => {
  test("pending while nothing has answered", async () => {
    const { data } = server(true);
    await data.queryClient.query(instanceQuery(data));
    expect(probe(data, () => useSearch("bak")).status).toBe("pending");
  });

  test("people, hashtags and posts where the server offers full-text search", async () => {
    const { fake, data } = server(true);
    await searched(data, "bak", { posts: true });
    const found = probe(data, () => useSearch("bak"));
    expect(found.status).toBe("success");
    expect(found.accounts.items.map((a) => a.acct)).toEqual(["kobo@bread.example"]);
    expect(found.tags.items.map((t) => t.name)).toEqual(["baking"]);
    expect(found.posts?.items.map((s) => s.id)).toEqual(["sam-poll", "sam-bread"].map(fake.idOf));
    expect(found.remote).toBeUndefined();
  });

  test("nothing of posts where the server has no search engine, and their search is never asked", async () => {
    const { data, requests } = server(false);
    await searched(data, "bak");
    const found = probe(data, () => useSearch("bak"));
    expect(found.posts).toBeUndefined();
    expect(found.status).toBe("success");
    expect(asked(requests, "/statuses/search")).toEqual([]);
  });

  test("a handle puts the account looked up first, before those the search found", async () => {
    const { data } = server(false);
    await searched(data, "@kiri@far.example", { remote: "kiri@far.example" });
    const found = probe(data, () => useSearch("@kiri@far.example"));
    expect(found.remote).toEqual({ acct: "kiri@far.example", status: "success", found: true });
    expect(found.accounts.items[0]?.acct).toBe("kiri@far.example");
  });

  test("a handle no server has is looked up and not found", async () => {
    const { data } = server(false);
    await searched(data, "@nobody@far.example", { remote: "nobody@far.example" });
    const found = probe(data, () => useSearch("@nobody@far.example"));
    expect(found.remote?.found).toBe(false);
    expect(found.remote?.status).not.toBe("pending");
    expect(found.accounts.items.map((a) => a.acct)).not.toContain("nobody@far.example");
  });

  // A probe renders once and never fetches, and a query that failed with nothing found yet shows pending as it mounts
  // (TanStack fetches it again): the failure is the one of a kind asked again after it had answered.
  test("failed when one kind failed, and retry asks again only what failed", async () => {
    const { fake, data, requests } = server(true);
    await searched(data, "bak", { posts: true });
    const down = fake.script("GET /tags/search", { fail: 503 });
    await data.queryClient.query({ ...tagResultsQuery(data, "bak"), staleTime: 0 }).catch(() => undefined);
    const failed = probe(data, () => useSearch("bak"));
    expect(failed.status).toBe("error");
    expect(failed.tags.status).toBe("error");
    expect(failed.accounts.status).toBe("success");

    const before = requests.length;
    down.end();
    failed.retry();
    await Bun.sleep(5);
    expect(requests.slice(before).map((r) => r.split("?")[0])).toEqual(["GET /tags/search"]);
    const again = probe(data, () => useSearch("bak"));
    expect(again.status).toBe("success");
    expect(again.tags.items.map((t) => t.name)).toEqual(["baking"]);
  });
});
