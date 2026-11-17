// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { scenario, type FakeOptions, type FakePerson } from "@my/e2e/fake";
import { QueryObserver } from "@tanstack/react-query";
import { accountQuery } from "./accounts";
import type { AppData } from "./app-data";
import { accountSearchQuery, tagSearchQuery, useAccountSearch, useTagSearch } from "./completion";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

const ids = (items: readonly { id: string }[] | undefined) => items?.map((a) => a.id);

/**
 * Lets the probe see a failed search as the mounted list does: a first render asks a failed query again
 * (retryOnMount), which the one render of the probe shows as pending.
 */
function mounted(data: AppData): void {
  data.queryClient.setDefaultOptions({ queries: { retryOnMount: false } });
}

const sora = "sora@photos.example";
/** An account of another server whose handle starts with sora's whole handle. */
const longer = "sora@photos.example.org";

/** A server that knows `longer` beside the scenario's people, where the reader mutes sora or not. */
function handles(muted: boolean): FakeOptions {
  const person = scenario.people.find((p) => p.username === sora);
  if (person === undefined) throw new Error("test: the scenario has no sora");
  const other: FakePerson = { ...person, username: longer };
  return {
    seed: {
      people: [...scenario.people, other],
      restrictions: muted ? [{ kind: "mute", by: "yuki", target: sora }] : [],
    },
  };
}

describe("useAccountSearch", () => {
  test("asks the server for 8 accounts that start with the word, and keeps the server's order: the followed first", async () => {
    // The server puts hana, sam and layla, whom the reader follows, before marco, and leaves the blocked and the
    // muted out (ADR-0092 §1).
    const { fake, data } = fakeData();
    const asked = fake.script("GET /accounts/search");
    await data.queryClient.query(accountSearchQuery(data, "a"));
    const found = probe(data, () => useAccountSearch("a"));
    expect(ids(found.items)).toEqual(["hana", "sam", "layla", "marco"].map(fake.accountId));
    expect(found.failed).toBe(false);
    expect(asked.seen.map((s) => [s.url.searchParams.get("q"), s.url.searchParams.get("limit")])).toEqual([["a", "8"]]);
  });

  test("with no word, asks nothing and has nothing", () => {
    const { data, requests } = fakeData();
    expect(probe(data, () => useAccountSearch(undefined))).toEqual({ items: undefined, failed: false });
    expect(requests).toEqual([]);
  });

  test("puts first the account of a whole user@domain that the search did not find", async () => {
    // The search leaves out whom the reader mutes; the lookup of their whole handle finds them.
    const { fake, data } = fakeData(handles(true));
    await data.queryClient.query(accountSearchQuery(data, sora));
    await data.queryClient.query(accountQuery(data, sora));
    const found = probe(data, () => useAccountSearch(sora));
    expect(ids(found.items)).toEqual([sora, longer].map(fake.accountId));
  });

  test("does not put a whole user@domain first again when the search found it", async () => {
    const { fake, data } = fakeData(handles(false));
    await data.queryClient.query(accountSearchQuery(data, sora));
    await data.queryClient.query(accountQuery(data, sora));
    const found = probe(data, () => useAccountSearch(sora));
    // The server puts the account of the very handle first, and it shows once.
    expect(ids(found.items)).toEqual([sora, longer].map(fake.accountId));
  });

  test("a search that failed has nothing and says it failed", async () => {
    const { fake, data } = fakeData();
    fake.script("GET /accounts/search", { fail: 503 });
    mounted(data);
    await data.queryClient.query(accountSearchQuery(data, "m")).catch(() => undefined);
    expect(probe(data, () => useAccountSearch("m"))).toEqual({ items: undefined, failed: true });
  });
});

/** A server whose posts use #baking twice and #bagels once. */
const tagged: FakeOptions = {
  seed: {
    people: scenario.people,
    posts: ["Rolled #bagels this morning.", "Day 3 of #baking.", "More #baking tonight."].map((text, i) => ({
      key: `tagged-${String(i)}`,
      author: "sam",
      text,
      visibility: "public",
      language: "en",
    })),
  },
};

describe("useTagSearch", () => {
  test("asks the server for 8 hashtags that start with the word, and keeps its order: the most used first", async () => {
    const { fake, data } = fakeData(tagged);
    const asked = fake.script("GET /tags/search");
    await data.queryClient.query(tagSearchQuery(data, "ba"));
    const found = probe(data, () => useTagSearch("ba"));
    expect(found.items?.map((t) => t.name)).toEqual(["baking", "bagels"]);
    expect(found.failed).toBe(false);
    expect(asked.seen.map((s) => [s.url.searchParams.get("q"), s.url.searchParams.get("limit")])).toEqual([
      ["ba", "8"],
    ]);
  });

  test("with no word, asks nothing and has nothing", () => {
    const { data, requests } = fakeData(tagged);
    expect(probe(data, () => useTagSearch(undefined))).toEqual({ items: undefined, failed: false });
    expect(requests).toEqual([]);
  });

  test("a search that failed says it failed", async () => {
    const { fake, data } = fakeData(tagged);
    fake.script("GET /tags/search", { fail: 503 });
    mounted(data);
    await data.queryClient.query(tagSearchQuery(data, "ba")).catch(() => undefined);
    expect(probe(data, () => useTagSearch("ba")).failed).toBe(true);
  });
});

describe("tagSearchQuery", () => {
  test("while the same word typed longer is asked, the answer for the shorter stands in; another word's never does", async () => {
    const { fake, data } = fakeData(tagged);
    await data.queryClient.query(tagSearchQuery(data, "ba"));
    // The next answers do not come while the test looks.
    const held = fake.script("GET /tags/search", { hold: "request" });
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
    expect(longer.data?.items.map((t) => t.name)).toEqual(["baking", "bagels"]);
    expect(typed("写").data).toBeUndefined();
    held.end();
  });
});
