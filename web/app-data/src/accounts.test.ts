// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ProblemError } from "@mycroblog/mykit";
import { QueryObserver } from "@tanstack/react-query";
import {
  accountByIdQuery,
  accountQuery,
  accountStatusesQuery,
  cachedAccount,
  changeRelationship,
  loadAccount,
  accountNoteLimit,
  relationshipQuery,
  useAccount,
  useRelationship,
  writeAccountNote,
  type Account,
  type Relationship,
} from "./accounts";
import type { AppData } from "./app-data";
import { fakeAppData, json, page, status } from "./test/fake";
import { probe } from "./test/probe";
import { homeTimelineQuery } from "./timeline";

/** A server that answers a write of a relationship with `following`, and the reads with what they ask. */
function server() {
  return fakeAppData((request) => {
    const path = new URL(request.url).pathname;
    if (path.endsWith("/follow"))
      return json(200, { id: "a", following: request.method === "POST", requested: false, blocking: false });
    if (path.endsWith("/relationships")) return json(200, { items: [{ id: "a", following: false }] });
    if (path.endsWith("/lookup")) return json(200, { id: "a", acct: "alice", followers_count: 1 });
    return json(200, page(["1"]));
  });
}

test("a follow puts the relationship the server answered into the cache", async () => {
  const { data, requests } = server();
  const following = () =>
    data.queryClient.getQueryData<{ items: Relationship[] }>(relationshipQuery(data, "a").queryKey)?.items[0]
      ?.following;
  await data.queryClient.query(relationshipQuery(data, "a"));
  await changeRelationship(data, "a", "follow");
  expect(requests.at(-1)?.pathname).toBe("/api/v1/accounts/a/follow");
  expect(following()).toBe(true);
  await changeRelationship(data, "a", "unfollow");
  expect(following()).toBe(false);
});

test("a change of a relationship marks the account's counts, the home timeline and the account's posts", async () => {
  const { data } = server();
  const { queryClient } = data;
  await queryClient.query(accountQuery(data, "alice"));
  await queryClient.query(accountByIdQuery(data, "a"));
  await queryClient.infiniteQuery(homeTimelineQuery(data));
  await queryClient.infiniteQuery(accountStatusesQuery(data, "a", "posts"));
  await queryClient.infiniteQuery(accountStatusesQuery(data, "b", "posts"));
  await changeRelationship(data, "a", "follow");
  const marked = (key: readonly unknown[]) => queryClient.getQueryState(key)?.isInvalidated;
  expect(marked(accountQuery(data, "alice").queryKey)).toBe(true);
  expect(marked(accountByIdQuery(data, "a").queryKey)).toBe(true);
  expect(marked(homeTimelineQuery(data).queryKey)).toBe(true);
  expect(marked(accountStatusesQuery(data, "a", "posts").queryKey)).toBe(true);
  expect(marked(accountStatusesQuery(data, "b", "posts").queryKey)).toBe(false);
});

/** A server whose account reads answer `account` after `wait`, and the URLs of the requests it received. */
function slowServer(account: Partial<Account>, wait = 20) {
  return fakeAppData(async () => {
    await Bun.sleep(wait);
    return json(200, { username: "alice", domain: "example.com", ...account });
  });
}

const alice = (fields: Partial<Account> = {}) =>
  ({ id: "a", acct: "alice", username: "alice", domain: "example.com", ...fields }) as Account;

/** Puts a home timeline whose one post is by `author` into the cache, as fetched at `at`. */
function cacheHome(data: AppData, author: Account, at: number, id = "1") {
  const home = { pages: [{ items: [status(id, { account: author })] }], pageParams: [undefined] };
  data.queryClient.setQueryData(homeTimelineQuery(data).queryKey, home, { updatedAt: at });
}

test("a post's author shows at once, and its account is fetched in the background once the copy is stale", async () => {
  const { data, requests } = slowServer(alice({ followers_count: 9 }));
  cacheHome(data, alice({ followers_count: 1 }), Date.now() - 60_000);
  expect((await loadAccount(data, "alice")).followers_count).toBe(1);
  expect(requests).toHaveLength(0);
  const observer = new QueryObserver(data.queryClient, accountByIdQuery(data, "a"));
  const shown: (number | undefined)[] = [];
  const done = new Promise<void>((resolve) => {
    observer.subscribe((result) => {
      shown.push(result.data?.followers_count);
      if (result.isSuccess && !result.isFetching) resolve();
    });
  });
  expect(observer.getCurrentResult().data?.followers_count).toBe(1);
  await done;
  expect(requests.map((r) => r.pathname)).toEqual(["/api/v1/accounts/a"]);
  expect(shown.at(-1)).toBe(9);
});

test("a fresh copy is not fetched again", () => {
  const { data, requests } = slowServer(alice());
  cacheHome(data, alice(), Date.now());
  const observer = new QueryObserver(data.queryClient, accountByIdQuery(data, "a"));
  const unsubscribe = observer.subscribe(() => undefined);
  expect(observer.getCurrentResult().data?.id).toBe("a");
  expect(requests).toHaveLength(0);
  unsubscribe();
});

test("the newest copy is the first data", async () => {
  const { data } = slowServer(alice());
  cacheHome(data, alice({ followers_count: 1 }), Date.now() - 60_000);
  const boost = status("3", { account: alice({ followers_count: 2 }) });
  const local = {
    pages: [{ items: [status("2", { account: alice({ id: "b", acct: "bob", username: "bob" }), boost })] }],
  };
  data.queryClient.setQueryData(["get", "/timelines/public", {}], { ...local, pageParams: [undefined] });
  const account = await data.queryClient.query({ ...accountByIdQuery(data, "a"), staleTime: Infinity });
  expect(account.followers_count).toBe(2);
  expect(cachedAccount(data, "ALICE@example.com")?.followers_count).toBe(2);
});

test("without a copy, the loader waits for the server", async () => {
  const { data, requests } = slowServer(alice({ followers_count: 4 }));
  expect((await loadAccount(data, "alice")).followers_count).toBe(4);
  expect(requests.map((r) => r.pathname)).toEqual(["/api/v1/accounts/lookup"]);
  const other = await data.queryClient.query({ ...accountByIdQuery(data, "b"), staleTime: Infinity });
  expect(other.followers_count).toBe(4);
  expect(requests.map((r) => r.pathname).at(-1)).toBe("/api/v1/accounts/b");
});

test("an older copy in a timeline does not replace the account the query holds", async () => {
  const { data } = slowServer(alice({ followers_count: 5 }));
  await data.queryClient.query(accountByIdQuery(data, "a"));
  cacheHome(data, alice({ followers_count: 1 }), Date.now() - 60_000);
  const account = await data.queryClient.query({ ...accountByIdQuery(data, "a"), staleTime: Infinity });
  expect(account.followers_count).toBe(5);
  expect((await loadAccount(data, "alice")).followers_count).toBe(5);
});

test("the media of an account are its posts without replies, keyed apart from them", () => {
  const { data } = server();
  const posts = accountStatusesQuery(data, "a", "posts").queryKey;
  const media = accountStatusesQuery(data, "a", "media").queryKey;
  expect(posts).not.toEqual(media);
  expect(posts[2]).toEqual(media[2]);
});

describe("cachedAccount", () => {
  const bob = (fields: Partial<Account> = {}) =>
    alice({ id: "b", acct: "bob@remote.example", username: "bob", domain: "remote.example", ...fields });

  test("finds a local account by its acct or its username@domain, in any case", () => {
    const { data } = slowServer(alice());
    cacheHome(data, alice(), Date.now());
    for (const handle of ["alice", "ALICE", "alice@example.com", "Alice@Example.COM"])
      expect(cachedAccount(data, handle)?.id).toBe("a");
  });

  test("finds a remote account by its acct in any case, and not by its username alone", () => {
    const { data } = slowServer(alice());
    cacheHome(data, bob(), Date.now());
    for (const handle of ["bob@remote.example", "BOB@Remote.Example"])
      expect(cachedAccount(data, handle)?.id).toBe("b");
    expect(cachedAccount(data, "bob")).toBeUndefined();
  });

  test("does not take an account of another server, or one the cache does not hold", () => {
    const { data } = slowServer(alice());
    cacheHome(data, bob(), Date.now());
    expect(cachedAccount(data, "bob@other.example")).toBeUndefined();
    expect(cachedAccount(data, "alice")).toBeUndefined();
  });

  test("finds the author of a quoted post", () => {
    const { data } = slowServer(alice());
    const quoted = status("q", { account: bob({ followers_count: 3 }) });
    const post = status("1", { account: alice(), quote: { state: "accepted", quoted_status: quoted } });
    data.queryClient.setQueryData(homeTimelineQuery(data).queryKey, {
      pages: [{ items: [post] }],
      pageParams: [undefined],
    });
    expect(cachedAccount(data, "bob@remote.example")?.followers_count).toBe(3);
  });

  test("finds an account the cache holds from its lookup, or from its own query", async () => {
    const looked = slowServer(bob({ followers_count: 7 }));
    expect(cachedAccount(looked.data, "bob@remote.example")).toBeUndefined();
    await looked.data.queryClient.query(accountQuery(looked.data, "bob@remote.example"));
    expect(cachedAccount(looked.data, "bob@remote.example")?.followers_count).toBe(7);
    const fetched = slowServer(bob({ followers_count: 8 }));
    await fetched.data.queryClient.query(accountByIdQuery(fetched.data, "b"));
    expect(cachedAccount(fetched.data, "bob@remote.example")?.followers_count).toBe(8);
  });
});

/**
 * A server that keeps the viewer's relationship with the account `a` (`relationship` at the start), changes it as the
 * writes of a note, a subscription and a follower ask, refuses a note longer than the limit, and records each write.
 */
function relationships(relationship: Partial<Relationship> = {}) {
  let now: Relationship = { id: "a", following: true, followed_by: true, notify: false, note: "", ...relationship };
  const writes: { method: string; path: string; body: unknown }[] = [];
  const fake = fakeAppData(async (request) => {
    const path = new URL(request.url).pathname;
    if (path === "/api/v1/accounts/relationships") return json(200, { items: [now] });
    const body = request.method === "PUT" ? ((await request.json()) as unknown) : undefined;
    writes.push({ method: request.method, path, body });
    if (path.endsWith("/note")) {
      const note = request.method === "PUT" ? (body as { note: string }).note : "";
      if (Array.from(note).length > accountNoteLimit)
        return json(422, { type: "about:blank", title: "Unprocessable", status: 422 });
      now = { ...now, note };
    } else if (path.endsWith("/subscribe")) now = { ...now, notify: request.method === "POST" };
    else if (path.endsWith("/follower")) now = { ...now, followed_by: false };
    return json(200, now);
  });
  return { ...fake, writes };
}

describe("the viewer's relationship with an account", () => {
  const shown = (data: AppData) => probe(data, () => useRelationship("a"));

  for (const [change, method, path, field, value] of [
    ["subscribe", "POST", "/api/v1/accounts/a/subscribe", "notify", true],
    ["unsubscribe", "DELETE", "/api/v1/accounts/a/subscribe", "notify", false],
    ["remove-follower", "DELETE", "/api/v1/accounts/a/follower", "followed_by", false],
  ] as const)
    test(`${change} sends ${method} ${path}, and the cache holds the relationship the server answered`, async () => {
      const { data, writes } = relationships({ notify: change === "unsubscribe" });
      await data.queryClient.query(relationshipQuery(data, "a"));
      await changeRelationship(data, "a", change);
      expect(writes).toEqual([{ method, path, body: undefined }]);
      expect(shown(data)?.[field]).toBe(value);
    });

  test("a private note is written, and the cache holds it once the write resolves", async () => {
    const { data, writes } = relationships();
    await data.queryClient.query(relationshipQuery(data, "a"));
    await writeAccountNote(data, "a", "Met at the bakery.");
    expect(writes).toEqual([{ method: "PUT", path: "/api/v1/accounts/a/note", body: { note: "Met at the bakery." } }]);
    expect(shown(data)?.note).toBe("Met at the bakery.");
  });

  test("an empty note deletes the note", async () => {
    const { data, writes } = relationships({ note: "Met at the bakery." });
    await data.queryClient.query(relationshipQuery(data, "a"));
    await writeAccountNote(data, "a", "");
    expect(writes).toEqual([{ method: "DELETE", path: "/api/v1/accounts/a/note", body: undefined }]);
    expect(shown(data)?.note).toBe("");
  });

  test("a note of the limit's characters (code points) is written; one more is refused, and the note stays", async () => {
    expect(accountNoteLimit).toBe(2000);
    const { data } = relationships({ note: "before" });
    await data.queryClient.query(relationshipQuery(data, "a"));
    const full = "🍞".repeat(accountNoteLimit);
    await writeAccountNote(data, "a", full);
    expect(shown(data)?.note).toBe(full);
    const refused = await writeAccountNote(data, "a", `${full}!`).then(
      () => undefined,
      (error: unknown) => error,
    );
    expect(refused).toBeInstanceOf(ProblemError);
    expect(shown(data)?.note).toBe(full);
  });
});

describe("useAccount", () => {
  test("shows a post's author from the cache at once, with no request", () => {
    const { data, requests } = slowServer(alice({ followers_count: 9 }));
    cacheHome(data, alice({ followers_count: 1 }), Date.now() - 60_000);
    expect(probe(data, () => useAccount("alice")).account?.followers_count).toBe(1);
    expect(requests).toHaveLength(0);
  });

  test("without a handle, has nothing and asks nothing", () => {
    const { data, requests } = slowServer(alice());
    expect(probe(data, () => useAccount(undefined))).toEqual({ account: undefined, error: null });
    expect(requests).toHaveLength(0);
  });

  test("without a copy, moves from the lookup to the account's query without another request", async () => {
    const { data, requests } = slowServer(alice({ followers_count: 4 }));
    expect(probe(data, () => useAccount("alice")).account).toBeUndefined();
    await loadAccount(data, "alice");
    expect(probe(data, () => useAccount("alice")).account?.followers_count).toBe(4);
    // The account's query starts from the lookup's answer, as fetched when it was: fresh, so a mount fetches nothing.
    const byId = data.queryClient.getQueryState<Account>(accountByIdQuery(data, "a").queryKey);
    const lookup = data.queryClient.getQueryState(accountQuery(data, "alice").queryKey);
    expect(byId?.data?.followers_count).toBe(4);
    expect(byId?.dataUpdatedAt).toBe(lookup?.dataUpdatedAt ?? Number.NaN);
    expect(byId?.isInvalidated).toBe(false);
    expect(requests.map((r) => r.pathname)).toEqual(["/api/v1/accounts/lookup"]);
  });
});
