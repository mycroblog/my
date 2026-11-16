// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { useRelationship } from "./accounts";
import { accountFollowsQuery, useAccountFollows, type FollowsList } from "./follows";
import { fakeAppData, json } from "./test/fake";
import { probe } from "./test/probe";

/**
 * A server whose account `a` has the lists `pages` (each page the IDs of its accounts, the next page's cursor its index),
 * answers the relationships of the IDs asked with `following` set for the IDs in `followed`, and answers a list with
 * `refuse` (a status and a problem type) instead when given.
 */
function server(pages: string[][], followed: string[] = [], refuse?: { status: number; type: string }) {
  return fakeAppData((request) => {
    const url = new URL(request.url);
    if (url.pathname === "/api/v1/accounts/relationships")
      return json(200, {
        items: url.searchParams.getAll("id").map((id) => ({ id, following: followed.includes(id) })),
      });
    if (refuse !== undefined)
      return json(refuse.status, { type: refuse.type, title: "Refused", status: refuse.status });
    const at = Number(url.searchParams.get("cursor") ?? "0");
    const items = (pages[at] ?? []).map((id) => ({ id, acct: id, username: id, display_name: id }));
    return json(200, { items, ...(at + 1 < pages.length ? { next_cursor: String(at + 1) } : {}) });
  });
}

const followsHidden = { status: 403, type: "/api/v1/problems/follows-hidden" };

/**
 * Fetches the list once, which fails, and keeps its failure for the probe as a screen that fetched it holds it: a
 * render alone would ask again (retryOnMount) and show the list pending.
 */
async function failed(data: ReturnType<typeof server>["data"], list: FollowsList) {
  const query = accountFollowsQuery(data, "a", list);
  data.queryClient.setQueryDefaults(query.queryKey, { retryOnMount: false });
  await data.queryClient.infiniteQuery(query).catch(() => undefined);
}

for (const list of ["followers", "following"] as const satisfies FollowsList[])
  describe(`the ${list} of an account`, () => {
    test("reads the list's pages in order, and the viewer's relationships with each page's accounts in one request", async () => {
      const { data, requests } = server([["b", "c"], ["d"]], ["c", "d"]);
      await data.queryClient.infiniteQuery({ ...accountFollowsQuery(data, "a", list), pages: 2 });
      const shown = probe(data, () => useAccountFollows("a", list));
      expect(shown.items.map((a) => a.id)).toEqual(["b", "c", "d"]);
      expect(shown.hasNextPage).toBe(false);
      expect(shown.hidden).toBe(false);
      const lists = requests.filter((r) => r.pathname === `/api/v1/accounts/a/${list}`);
      expect(lists.map((r) => r.searchParams.get("cursor"))).toEqual([null, "1"]);
      const relationships = requests.filter((r) => r.pathname === "/api/v1/accounts/relationships");
      expect(relationships.map((r) => r.searchParams.getAll("id"))).toEqual([["b", "c"], ["d"]]);
      // Each row's follow shows from the page's relationships, with no request of its own.
      const before = requests.length;
      expect(probe(data, () => useRelationship("b"))?.following).toBe(false);
      expect(probe(data, () => useRelationship("c"))?.following).toBe(true);
      expect(probe(data, () => useRelationship("d"))?.following).toBe(true);
      expect(requests).toHaveLength(before);
    });

    test("says there is a next page until the last one is read", async () => {
      const { data } = server([["b"], ["c"]]);
      await data.queryClient.infiniteQuery(accountFollowsQuery(data, "a", list));
      const shown = probe(data, () => useAccountFollows("a", list));
      expect(shown.items.map((a) => a.id)).toEqual(["b"]);
      expect(shown.hasNextPage).toBe(true);
    });

    test("an empty list shows no account, and is neither hidden nor failed", async () => {
      const { data, requests } = server([[]]);
      await data.queryClient.infiniteQuery(accountFollowsQuery(data, "a", list));
      const shown = probe(data, () => useAccountFollows("a", list));
      expect(shown.items).toEqual([]);
      expect(shown.status).toBe("success");
      expect(shown.hidden).toBe(false);
      // No account, so no relationship to read.
      expect(requests.map((r) => r.pathname)).not.toContain("/api/v1/accounts/relationships");
    });

    test("a list its owner keeps from the viewer is hidden, with no account", async () => {
      const { data } = server([["b"]], [], followsHidden);
      await failed(data, list);
      const shown = probe(data, () => useAccountFollows("a", list));
      expect(shown.hidden).toBe(true);
      expect(shown.items).toEqual([]);
    });

    test("another failure is a failure, not a hidden list", async () => {
      const { data } = server([["b"]], [], { status: 500, type: "about:blank" });
      await failed(data, list);
      const shown = probe(data, () => useAccountFollows("a", list));
      expect(shown.hidden).toBe(false);
      expect(shown.status).toBe("error");
      expect(shown.error).not.toBeNull();
    });
  });

test("the two lists of an account are kept apart", () => {
  const { data } = server([]);
  expect(accountFollowsQuery(data, "a", "followers").queryKey).not.toEqual(
    accountFollowsQuery(data, "a", "following").queryKey,
  );
});
