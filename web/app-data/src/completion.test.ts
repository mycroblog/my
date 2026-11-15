// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { QueryObserver } from "@tanstack/react-query";
import { accountQuery, type Account } from "./accounts";
import type { AppData } from "./app-data";
import { accountSearchQuery, tagSearchQuery, useAccountSearch, useTagSearch, type TagSuggestion } from "./completion";
import { fakeAppData, json } from "./test/fake";
import { probe } from "./test/probe";

const account = (acct: string) =>
  ({
    id: acct,
    username: acct.split("@")[0],
    acct,
    display_name: acct,
    url: `https://example.com/@${acct}`,
  }) as Account;

const tag = (name: string, uses: number) => ({ name, uses }) as unknown as TagSuggestion;

const ids = (items: readonly { id: string }[] | undefined) => items?.map((a) => a.id);

const unavailable = { type: "about:blank", title: "Service Unavailable", status: 503 };

/**
 * Lets the probe see a failed search as the mounted list does: a first render asks a failed query again
 * (retryOnMount), which the one render of the probe shows as pending.
 */
function mounted(data: AppData): void {
  data.queryClient.setDefaultOptions({ queries: { retryOnMount: false } });
}

describe("useAccountSearch", () => {
  test("asks the server for 8 accounts that start with the word, and keeps the server's order: the followed first", async () => {
    // The server puts mio, whom the reader follows, before marco, and leaves the blocked and the muted out (ADR-0092 §1).
    const { data, requests } = fakeAppData(() => json(200, { items: [account("mio"), account("marco")] }));
    await data.queryClient.query(accountSearchQuery(data, "m"));
    const found = probe(data, () => useAccountSearch("m"));
    expect(ids(found.items)).toEqual(["mio", "marco"]);
    expect(found.failed).toBe(false);
    expect(requests.map((u) => [u.pathname, u.searchParams.get("q"), u.searchParams.get("limit")])).toEqual([
      ["/api/v1/accounts/search", "m", "8"],
    ]);
  });

  test("with no word, asks nothing and has nothing", () => {
    const { data, requests } = fakeAppData(() => json(200, { items: [] }));
    expect(probe(data, () => useAccountSearch(undefined))).toEqual({ items: undefined, failed: false });
    expect(requests).toEqual([]);
  });

  test("puts first the account of a whole user@domain that the search did not find", async () => {
    const { data } = fakeAppData((request) =>
      new URL(request.url).pathname.endsWith("/accounts/search")
        ? json(200, { items: [account("samuel@other.example")] })
        : json(200, account("sam@other.example")),
    );
    await data.queryClient.query(accountSearchQuery(data, "sam@other.example"));
    await data.queryClient.query(accountQuery(data, "sam@other.example"));
    const found = probe(data, () => useAccountSearch("sam@other.example"));
    expect(ids(found.items)).toEqual(["sam@other.example", "samuel@other.example"]);
  });

  test("does not put a whole user@domain first again when the search found it", async () => {
    const { data } = fakeAppData((request) =>
      new URL(request.url).pathname.endsWith("/accounts/search")
        ? json(200, { items: [account("Sam@other.example"), account("samuel@other.example")] })
        : json(200, account("sam@other.example")),
    );
    await data.queryClient.query(accountSearchQuery(data, "sam@other.example"));
    await data.queryClient.query(accountQuery(data, "sam@other.example"));
    const found = probe(data, () => useAccountSearch("sam@other.example"));
    expect(ids(found.items)).toEqual(["Sam@other.example", "samuel@other.example"]);
  });

  test("a search that failed has nothing and says it failed", async () => {
    const { data } = fakeAppData(() => json(503, unavailable));
    mounted(data);
    await data.queryClient.query(accountSearchQuery(data, "m")).catch(() => undefined);
    expect(probe(data, () => useAccountSearch("m"))).toEqual({ items: undefined, failed: true });
  });
});

describe("useTagSearch", () => {
  test("asks the server for 8 hashtags that start with the word, and keeps its order: the most used first", async () => {
    const { data, requests } = fakeAppData(() => json(200, { items: [tag("baking", 12), tag("bagels", 3)] }));
    await data.queryClient.query(tagSearchQuery(data, "ba"));
    const found = probe(data, () => useTagSearch("ba"));
    expect(found.items?.map((t) => t.name)).toEqual(["baking", "bagels"]);
    expect(found.failed).toBe(false);
    expect(requests.map((u) => [u.pathname, u.searchParams.get("q"), u.searchParams.get("limit")])).toEqual([
      ["/api/v1/tags/search", "ba", "8"],
    ]);
  });

  test("with no word, asks nothing and has nothing", () => {
    const { data, requests } = fakeAppData(() => json(200, { items: [] }));
    expect(probe(data, () => useTagSearch(undefined))).toEqual({ items: undefined, failed: false });
    expect(requests).toEqual([]);
  });

  test("a search that failed says it failed", async () => {
    const { data } = fakeAppData(() => json(503, unavailable));
    mounted(data);
    await data.queryClient.query(tagSearchQuery(data, "ba")).catch(() => undefined);
    expect(probe(data, () => useTagSearch("ba")).failed).toBe(true);
  });
});

describe("tagSearchQuery", () => {
  test("while the same word typed longer is asked, the answer for the shorter stands in; another word's never does", () => {
    const { data } = fakeAppData(() => new Promise<Response>(() => undefined));
    data.queryClient.setQueryData(tagSearchQuery(data, "ba").queryKey, { items: [tag("baking", 12)] });
    /** What an observer of the answer for "ba" shows once it is asked for `next` instead. */
    const typed = (next: string) => {
      const observer = new QueryObserver(data.queryClient, tagSearchQuery(data, "ba"));
      const unsubscribe = observer.subscribe(() => undefined);
      observer.setOptions(tagSearchQuery(data, next));
      const result = observer.getCurrentResult();
      unsubscribe();
      return result;
    };
    const longer = typed("bak");
    expect(longer.isPlaceholderData).toBe(true);
    expect(longer.data?.items.map((t) => t.name)).toEqual(["baking"]);
    expect(typed("写").data).toBeUndefined();
  });
});
