// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { scenario, type FakeOptions } from "@my/e2e/fake";
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
} from "./lists";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

/**
 * A server of the reader's lists: `bread` holding sam and mio, and `work` holding nobody. The reader follows sam, mio
 * and layla, not marco; with `listLimits` the server refuses a list or a member past them, as the real one does at 50
 * and 1,000. `id` is the ID of the account of a person, `account` the account as the server gives it.
 */
function server(listLimits: NonNullable<FakeOptions["listLimits"]> = {}) {
  const { fake, data, requests } = fakeData({
    seed: {
      people: scenario.people,
      follows: ["sam", "mio", "layla"].map((followee) => ({ follower: "yuki", followee })),
      lists: [
        { key: "bread", owner: "yuki", name: "Bread and coffee", members: ["sam", "mio"] },
        { key: "work", owner: "yuki", name: "Work", members: [] },
      ],
    },
    listLimits,
  });
  const account = async (username: string): Promise<Account> => {
    const { data: found } = await data.api.GET("/accounts/{id}", {
      params: { path: { id: fake.accountId(username) } },
    });
    if (found === undefined) throw new Error(`test: the server has no ${username}`);
    return found;
  };
  return {
    data,
    requests,
    bread: fake.listPath(0).replace("/lists/", ""),
    work: fake.listPath(1).replace("/lists/", ""),
    id: fake.accountId,
    account,
  };
}

/** Reads what the screens of the lists read: the index, the list `bread` and its members, and the lists of `of`. */
async function read(data: AppData, bread: string, ...of: string[]) {
  await data.queryClient.query(listsQuery(data));
  await data.queryClient.query(listQuery(data, bread));
  await data.queryClient.infiniteQuery(listMembersQuery(data, bread));
  for (const id of of) await data.queryClient.query(accountListsQuery(data, id));
}

const writes = (data: AppData) => probe(data, useListWrites);
const index = (data: AppData) => probe(data, () => useLists()).lists.map((l) => [l.name, l.members_count]);
const membersOf = (data: AppData, id: string) => probe(data, () => useListMembers(id)).items.map((a) => a.id);
const countOf = (data: AppData, id: string) => probe(data, () => useList(id)).list?.members_count;
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
    const { data, requests, bread } = server();
    await read(data, bread);
    const made = await writes(data).create("Friends");
    expect(requests).toContain("POST /lists");
    expect(made.name).toBe("Friends");
    expect(made.members_count).toBe(0);
    expect(index(data)).toContainEqual(["Friends", 0]);
    expect(index(data)).toHaveLength(3);
  });

  test("a list past the limit of lists is refused as that limit, and the index is as it was", async () => {
    const { data, bread } = server({ lists: 2 });
    await read(data, bread);
    const error = await outcome(writes(data).create("Friends"));
    expect(listLimitOf(error)).toBe("lists");
    expect(index(data)).toEqual([
      ["Bread and coffee", 2],
      ["Work", 0],
    ]);
  });

  test("a list renamed shows its new name on its screen and in the index at once", async () => {
    const { data, requests, bread } = server();
    await read(data, bread);
    await writes(data).rename(bread, "Bakers");
    expect(requests).toContain(`PATCH /lists/${bread}`);
    expect(probe(data, () => useList(bread)).list?.name).toBe("Bakers");
    expect(index(data)).toEqual([
      ["Bakers", 2],
      ["Work", 0],
    ]);
  });

  test("a list deleted leaves the index at once", async () => {
    const { data, requests, bread } = server();
    await read(data, bread);
    await writes(data).remove(bread);
    expect(requests).toContain(`DELETE /lists/${bread}`);
    expect(index(data)).toEqual([["Work", 0]]);
  });

  test("a list the server no longer has is gone, not failed", async () => {
    const { data } = server();
    const deleted = "00000000-0000-7000-8000-00000000ffff";
    const query = listQuery(data, deleted);
    data.queryClient.setQueryDefaults(query.queryKey, { retryOnMount: false });
    await data.queryClient.query(query).catch(() => undefined);
    expect(probe(data, () => useList(deleted)).status).toBe("gone");
  });
});

describe("a list's members", () => {
  test("an account added comes first among the members, counts on the list and the index, and holds the list, at once", async () => {
    const { data, requests, bread, id, account } = server();
    await read(data, bread, id("layla"));
    expect(holding(data, id("layla"))).toEqual([]);
    await writes(data).member(bread, await account("layla"), true);
    expect(requests).toContain(`POST /lists/${bread}/accounts/${id("layla")}`);
    expect(membersOf(data, bread)[0]).toBe(id("layla"));
    expect(membersOf(data, bread)).toHaveLength(3);
    expect(countOf(data, bread)).toBe(3);
    expect(index(data)).toContainEqual(["Bread and coffee", 3]);
    expect(holding(data, id("layla"))).toEqual([bread]);
  });

  test("an account taken out leaves the members, the counts and its lists at once", async () => {
    const { data, requests, bread, id, account } = server();
    await read(data, bread, id("sam"));
    expect(holding(data, id("sam"))).toEqual([bread]);
    await writes(data).member(bread, await account("sam"), false);
    expect(requests).toContain(`DELETE /lists/${bread}/accounts/${id("sam")}`);
    expect(membersOf(data, bread)).toEqual([id("mio")]);
    expect(countOf(data, bread)).toBe(1);
    expect(index(data)).toContainEqual(["Bread and coffee", 1]);
    expect(holding(data, id("sam"))).toEqual([]);
  });

  test("a member added again is counted once", async () => {
    const { data, bread, id, account } = server();
    await read(data, bread, id("sam"));
    await writes(data).member(bread, await account("sam"), true);
    expect(membersOf(data, bread).filter((member) => member === id("sam"))).toHaveLength(1);
    expect(countOf(data, bread)).toBe(2);
    expect(index(data)).toContainEqual(["Bread and coffee", 2]);
  });

  test("an account added from a profile, its list's members unread, counts on the list and holds it", async () => {
    const { data, work, id, account } = server();
    await data.queryClient.query(listsQuery(data));
    await data.queryClient.query(accountListsQuery(data, id("layla")));
    await writes(data).member(work, await account("layla"), true);
    expect(holding(data, id("layla"))).toEqual([work]);
    expect(index(data)).toContainEqual(["Work", 1]);
  });

  test("a member past the limit of members is refused as that limit, and nothing changes", async () => {
    const { data, bread, id, account } = server({ members: 2 });
    await read(data, bread, id("layla"));
    const error = await outcome(writes(data).member(bread, await account("layla"), true));
    expect(listLimitOf(error)).toBe("members");
    expect(membersOf(data, bread)).toEqual([id("sam"), id("mio")]);
    expect(countOf(data, bread)).toBe(2);
    expect(holding(data, id("layla"))).toEqual([]);
  });

  test("an account the reader does not follow is refused, but not as a limit", async () => {
    const { data, bread, id, account } = server();
    await read(data, bread, id("marco"));
    const error = await outcome(writes(data).member(bread, await account("marco"), true));
    expect(error).not.toBe("resolved");
    expect(listLimitOf(error)).toBeUndefined();
    expect(membersOf(data, bread)).toEqual([id("sam"), id("mio")]);
    expect(holding(data, id("marco"))).toEqual([]);
  });
});

describe("finding an account the reader follows", () => {
  test("nothing is asked for an empty query", () => {
    const { data, requests } = server();
    expect(probe(data, () => useFollowedSearch("  ")).items).toBeUndefined();
    expect(requests).toEqual([]);
  });

  test("of the accounts found, only those the reader follows, each with its relationship read", async () => {
    // The server finds sam, mio and marco for "m"; the reader does not follow marco.
    const { data, requests, id } = server();
    await data.queryClient.query(followedSearchQuery(data, "m"));
    const found = probe(data, () => useFollowedSearch("m"));
    expect(found.items?.map((a) => a.id)).toEqual([id("sam"), id("mio")]);
    expect(found.failed).toBe(false);
    const before = requests.length;
    expect(probe(data, () => useRelationship(id("mio")))?.following).toBe(true);
    expect(probe(data, () => useRelationship(id("marco")))?.following).toBe(false);
    expect(requests).toHaveLength(before);
  });
});
