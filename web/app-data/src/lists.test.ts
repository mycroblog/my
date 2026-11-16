// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { useRelationship, type Account } from "./accounts";
import type { AppData } from "./app-data";
import {
  accountListsQuery,
  followedSearchQuery,
  listLimitOf,
  listMembersQuery,
  listQuery,
  listsQuery,
  useAccountLists,
  useFollowedSearch,
  useList,
  useListMembers,
  useLists,
  useListWrites,
  type List,
} from "./lists";
import { fakeAppData, json } from "./test/fake";
import { probe } from "./test/probe";

/** An account with the ID given as its every name; the rest are left out. */
const account = (id: string) => ({ id, acct: id, username: id, display_name: id }) as Account;

const problem = (status: number, type: string) => json(status, { type, title: "Refused", status });

/**
 * A server of the reader's lists: "bread" holding sam and mio, and "work" holding nobody. The reader follows sam, mio and
 * layla, not marco; the server refuses a list past `lists` and a member past `members`, as the real one does at 50 and
 * 1,000, and refuses to add an account the reader does not follow.
 */
function server({ lists: maxLists = 50, members: maxMembers = 1000 } = {}) {
  const followed = new Set(["sam", "mio", "layla"]);
  const lists = new Map<string, { name: string; members: string[] }>([
    ["bread", { name: "Bread and coffee", members: ["sam", "mio"] }],
    ["work", { name: "Work", members: [] }],
  ]);
  const shown = (id: string): List => {
    const l = lists.get(id);
    return { id, name: l?.name ?? "", members_count: l?.members.length ?? 0, created_at: "2026-11-16T00:00:00Z" };
  };
  let made = 0;
  const sent: string[] = [];
  const fake = fakeAppData(async (r) => {
    const url = new URL(r.url);
    const path = url.pathname.replace("/api/v1", "");
    sent.push(`${r.method} ${path}`);
    const body = r.method === "POST" || r.method === "PATCH" ? await r.text() : "";
    const name = body === "" ? undefined : (JSON.parse(body) as { name?: string }).name;
    if (path === "/lists" && r.method === "GET") return json(200, { items: [...lists.keys()].map(shown) });
    if (path === "/lists" && r.method === "POST") {
      if (lists.size >= maxLists) return problem(422, "/api/v1/problems/too-many-lists");
      const id = `new${String(++made)}`;
      lists.set(id, { name: name ?? "", members: [] });
      return json(201, shown(id));
    }
    const one = /^\/lists\/([^/]+)$/.exec(path)?.[1];
    if (one !== undefined) {
      const l = lists.get(one);
      if (l === undefined) return problem(404, "about:blank");
      if (r.method === "PATCH") l.name = name ?? l.name;
      if (r.method === "DELETE") {
        lists.delete(one);
        return new Response(null, { status: 204 });
      }
      return json(200, shown(one));
    }
    const members = /^\/lists\/([^/]+)\/accounts$/.exec(path)?.[1];
    if (members !== undefined) return json(200, { items: (lists.get(members)?.members ?? []).map(account) });
    const member = /^\/lists\/([^/]+)\/accounts\/([^/]+)$/.exec(path);
    if (member) {
      const l = lists.get(member[1] ?? "");
      const who = member[2] ?? "";
      if (l === undefined) return problem(404, "about:blank");
      if (r.method === "POST") {
        if (!followed.has(who)) return problem(422, "/api/v1/problems/not-followed");
        if (!l.members.includes(who)) {
          if (l.members.length >= maxMembers) return problem(422, "/api/v1/problems/too-many-list-members");
          l.members.push(who);
        }
      } else l.members = l.members.filter((m) => m !== who);
      return new Response(null, { status: 204 });
    }
    const of = /^\/accounts\/([^/]+)\/lists$/.exec(path)?.[1];
    if (of !== undefined)
      return json(200, { items: [...lists.keys()].filter((id) => lists.get(id)?.members.includes(of)).map(shown) });
    if (path === "/accounts/search")
      return json(200, {
        items: ["layla", "lars"].filter((id) => id.startsWith(url.searchParams.get("q") ?? "")).map(account),
      });
    if (path === "/accounts/relationships")
      return json(200, {
        items: url.searchParams.getAll("id").map((id) => ({ id, following: followed.has(id) })),
      });
    return problem(404, "about:blank");
  });
  return { ...fake, sent };
}

/** Reads what the screens of the lists read: the index, the list "bread" and its members, and the lists of `of`. */
async function read(data: AppData, ...of: string[]) {
  await data.queryClient.query(listsQuery(data));
  await data.queryClient.query(listQuery(data, "bread"));
  await data.queryClient.infiniteQuery(listMembersQuery(data, "bread"));
  for (const id of of) await data.queryClient.query(accountListsQuery(data, id));
}

const writes = (data: AppData) => probe(data, useListWrites);
const index = (data: AppData) => probe(data, () => useLists()).lists.map((l) => [l.name, l.members_count]);
const membersOf = (data: AppData, id = "bread") => probe(data, () => useListMembers(id)).items.map((a) => a.id);
const countOf = (data: AppData, id = "bread") => probe(data, () => useList(id)).list?.members_count;
const holding = (data: AppData, id: string) => {
  const lists = probe(data, () => useAccountLists(id, true));
  return lists === undefined ? undefined : [...lists].sort();
};

/** How a write ended: the error it rejected with, or "resolved". */
const outcome = (write: Promise<unknown>) =>
  write.then(
    () => "resolved" as const,
    (error: unknown) => error,
  );

describe("a list made, renamed and deleted", () => {
  test("a list made joins the index at once, empty", async () => {
    const { data, sent } = server();
    await read(data);
    const made = await writes(data).create("Friends");
    expect(sent).toContain("POST /lists");
    expect(made.name).toBe("Friends");
    expect(made.members_count).toBe(0);
    expect(index(data)).toContainEqual(["Friends", 0]);
    expect(index(data)).toHaveLength(3);
  });

  test("a list past the limit of lists is refused as that limit, and the index is as it was", async () => {
    const { data } = server({ lists: 2 });
    await read(data);
    const error = await outcome(writes(data).create("Friends"));
    expect(listLimitOf(error)).toBe("lists");
    expect(index(data)).toEqual([
      ["Bread and coffee", 2],
      ["Work", 0],
    ]);
  });

  test("a list renamed shows its new name on its screen and in the index at once", async () => {
    const { data, sent } = server();
    await read(data);
    await writes(data).rename("bread", "Bakers");
    expect(sent).toContain("PATCH /lists/bread");
    expect(probe(data, () => useList("bread")).list?.name).toBe("Bakers");
    expect(index(data)).toEqual([
      ["Bakers", 2],
      ["Work", 0],
    ]);
  });

  test("a list deleted leaves the index at once", async () => {
    const { data, sent } = server();
    await read(data);
    await writes(data).remove("bread");
    expect(sent).toContain("DELETE /lists/bread");
    expect(index(data)).toEqual([["Work", 0]]);
  });

  test("a list the server no longer has is gone, not failed", async () => {
    const { data } = server();
    const query = listQuery(data, "deleted");
    data.queryClient.setQueryDefaults(query.queryKey, { retryOnMount: false });
    await data.queryClient.query(query).catch(() => undefined);
    expect(probe(data, () => useList("deleted")).status).toBe("gone");
  });
});

describe("a list's members", () => {
  test("an account added comes first among the members, counts on the list and the index, and holds the list, at once", async () => {
    const { data, sent } = server();
    await read(data, "layla");
    expect(holding(data, "layla")).toEqual([]);
    await writes(data).member("bread", account("layla"), true);
    expect(sent).toContain("POST /lists/bread/accounts/layla");
    expect(membersOf(data)[0]).toBe("layla");
    expect(membersOf(data)).toHaveLength(3);
    expect(countOf(data)).toBe(3);
    expect(index(data)).toContainEqual(["Bread and coffee", 3]);
    expect(holding(data, "layla")).toEqual(["bread"]);
  });

  test("an account taken out leaves the members, the counts and its lists at once", async () => {
    const { data, sent } = server();
    await read(data, "sam");
    expect(holding(data, "sam")).toEqual(["bread"]);
    await writes(data).member("bread", account("sam"), false);
    expect(sent).toContain("DELETE /lists/bread/accounts/sam");
    expect(membersOf(data)).toEqual(["mio"]);
    expect(countOf(data)).toBe(1);
    expect(index(data)).toContainEqual(["Bread and coffee", 1]);
    expect(holding(data, "sam")).toEqual([]);
  });

  test("a member added again is counted once", async () => {
    const { data } = server();
    await read(data, "sam");
    await writes(data).member("bread", account("sam"), true);
    expect(membersOf(data).filter((id) => id === "sam")).toHaveLength(1);
    expect(countOf(data)).toBe(2);
    expect(index(data)).toContainEqual(["Bread and coffee", 2]);
  });

  test("an account added from a profile, its list's members unread, counts on the list and holds it", async () => {
    const { data } = server();
    await data.queryClient.query(listsQuery(data));
    await data.queryClient.query(accountListsQuery(data, "layla"));
    await writes(data).member("work", account("layla"), true);
    expect(holding(data, "layla")).toEqual(["work"]);
    expect(index(data)).toContainEqual(["Work", 1]);
  });

  test("a member past the limit of members is refused as that limit, and nothing changes", async () => {
    const { data } = server({ members: 2 });
    await read(data, "layla");
    const error = await outcome(writes(data).member("bread", account("layla"), true));
    expect(listLimitOf(error)).toBe("members");
    expect(membersOf(data)).toEqual(["sam", "mio"]);
    expect(countOf(data)).toBe(2);
    expect(holding(data, "layla")).toEqual([]);
  });

  test("an account the reader does not follow is refused, but not as a limit", async () => {
    const { data } = server();
    await read(data, "marco");
    const error = await outcome(writes(data).member("bread", account("marco"), true));
    expect(error).not.toBe("resolved");
    expect(listLimitOf(error)).toBeUndefined();
    expect(membersOf(data)).toEqual(["sam", "mio"]);
    expect(holding(data, "marco")).toEqual([]);
  });
});

describe("finding an account the reader follows", () => {
  test("nothing is asked for an empty query", () => {
    const { data, sent } = server();
    expect(probe(data, () => useFollowedSearch("  ")).items).toBeUndefined();
    expect(sent).toEqual([]);
  });

  test("of the accounts found, only those the reader follows, each with its relationship read", async () => {
    const { data, sent } = server();
    await data.queryClient.query(followedSearchQuery(data, "la"));
    const found = probe(data, () => useFollowedSearch("la"));
    expect(found.items?.map((a) => a.id)).toEqual(["layla"]);
    expect(found.failed).toBe(false);
    const before = sent.length;
    expect(probe(data, () => useRelationship("layla"))?.following).toBe(true);
    expect(sent).toHaveLength(before);
  });
});
