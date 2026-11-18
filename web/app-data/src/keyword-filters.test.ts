// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import { accountStatusesQuery, viewerQuery } from "./accounts";
import type { AppData } from "./app-data";
import {
  deleteKeywordFilter,
  filterContexts,
  keywordFilterQuery,
  keywordFiltersQuery,
  saveKeywordFilter,
  useKeywordFilter,
  useKeywordFilters,
  type KeywordFilterRequest,
} from "./keyword-filters";
import { notificationsQuery } from "./notifications";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { refusedWith } from "./test/refused";
import {
  bookmarksQuery,
  homeTimelineQuery,
  listTimelineQuery,
  publicTimelineQuery,
  tagTimelineQuery,
} from "./timeline";
import { statusContextQuery } from "./thread";

const request: KeywordFilterRequest = {
  title: "Sourdough",
  action: "hide",
  contexts: ["home", "public"],
  keywords: [{ keyword: "levain", whole_word: true }],
};

/** What a screen of the filters shows again: the cache while it holds the answer and it is not marked, else a read. */
async function listed(data: AppData) {
  const { items } = await data.queryClient.query({ ...keywordFiltersQuery(data), staleTime: Infinity });
  return items;
}

describe("filterContexts", () => {
  test("are the five places the specification names a filter applies", () => {
    expect([...filterContexts].sort()).toEqual(["account", "home", "notifications", "public", "thread"]);
  });
});

describe("useKeywordFilters", () => {
  test("is empty and pending until the filters are read, then lists them oldest first, an expired one marked", async () => {
    const { data } = fakeData();
    expect(probe(data, useKeywordFilters)).toMatchObject({ filters: [], status: "pending" });
    await data.queryClient.query(keywordFiltersQuery(data));
    const { filters, status } = probe(data, useKeywordFilters);
    expect(status).toBe("success");
    expect(filters.map((f) => [f.title, f.action, f.expired])).toEqual([
      ["Spoilers", "warn", false],
      ["Election season", "hide", false],
      ["Marathon week", "warn", true],
    ]);
  });

  test("a filter without an expiry never expires", async () => {
    const { data } = fakeData();
    await data.queryClient.query(keywordFiltersQuery(data));
    const [spoilers] = probe(data, useKeywordFilters).filters;
    expect(spoilers?.expires_at).toBeUndefined();
    expect(spoilers?.expired).toBe(false);
  });

  test("reports an error when the filters could not be read", async () => {
    const { fake, data } = fakeData();
    fake.script("GET /filters", { fail: 500 });
    await data.queryClient.query(keywordFiltersQuery(data)).catch(() => undefined);
    const queries = data.queryClient.getDefaultOptions().queries;
    data.queryClient.setDefaultOptions({ queries: { ...queries, retryOnMount: false } });
    expect(probe(data, useKeywordFilters)).toMatchObject({ filters: [], status: "error" });
  });
});

describe("useKeywordFilter", () => {
  test("is the filter once read, with its words and where it applies", async () => {
    const { data } = fakeData();
    const [spoilers] = await listed(data);
    const id = spoilers?.id ?? "";
    await data.queryClient.query(keywordFilterQuery(data, id));
    const { filter, error } = probe(data, () => useKeywordFilter(id));
    expect(error).toBeNull();
    expect(filter).toMatchObject({
      title: "Spoilers",
      contexts: ["home", "public", "thread"],
      keywords: [
        { keyword: "finale", whole_word: true },
        { keyword: "season 3", whole_word: false },
      ],
    });
  });

  test("a filter the server does not have is an error, not a filter", async () => {
    const { data } = fakeData();
    await data.queryClient.query(keywordFilterQuery(data, "gone")).catch(() => undefined);
    const queries = data.queryClient.getDefaultOptions().queries;
    data.queryClient.setDefaultOptions({ queries: { ...queries, retryOnMount: false } });
    const { filter, error } = probe(data, () => useKeywordFilter("gone"));
    expect(filter).toBeUndefined();
    expect(error).toBeInstanceOf(Error);
  });
});

describe("saveKeywordFilter", () => {
  test("creates a filter with a POST, and the list holds it", async () => {
    const { fake, data, requests } = fakeData();
    const sent = fake.script("POST /filters");
    await listed(data);
    const made = await saveKeywordFilter(data, request);
    expect(requests.filter((r) => !r.startsWith("GET "))).toEqual(["POST /filters"]);
    expect(sent.seen.map((s) => s.body)).toEqual([request]);
    expect(made).toMatchObject({ title: "Sourdough", action: "hide", expired: false });
    expect((await listed(data)).map((f) => f.title)).toEqual([
      "Spoilers",
      "Election season",
      "Marathon week",
      "Sourdough",
    ]);
  });

  test("changes a filter with a PATCH of its id, and the list and the filter's own copy hold the change", async () => {
    const { data, requests } = fakeData();
    const [spoilers] = await listed(data);
    const id = spoilers?.id ?? "";
    await data.queryClient.query(keywordFilterQuery(data, id));
    await saveKeywordFilter(data, { ...request, title: "Spoilers and more" }, id);
    expect(requests.filter((r) => !r.startsWith("GET "))).toEqual([`PATCH /filters/${id}`]);
    expect((await listed(data)).map((f) => f.title)).toEqual(["Spoilers and more", "Election season", "Marathon week"]);
    const copy = await data.queryClient.query({ ...keywordFilterQuery(data, id), staleTime: Infinity });
    expect(copy).toMatchObject({ id, title: "Spoilers and more", action: "hide" });
  });

  test("an expiry in the past makes the saved filter expired", async () => {
    const { data } = fakeData();
    const made = await saveKeywordFilter(data, { ...request, expires_at: "2000-01-01T00:00:00Z" });
    expect(made.expired).toBe(true);
  });

  test("rejects an invalid filter with a 422, and the list stays as it was", async () => {
    const { data } = fakeData();
    await listed(data);
    for (const bad of [
      { ...request, title: "  " },
      { ...request, contexts: [] },
      { ...request, keywords: [] },
    ]) {
      expect((await refusedWith(saveKeywordFilter(data, bad))).status).toBe(422);
    }
    expect(await listed(data)).toHaveLength(3);
  });

  test("a change of a filter that is gone is refused as not found", async () => {
    const { data } = fakeData();
    expect((await refusedWith(saveKeywordFilter(data, request, "gone"))).status).toBe(404);
  });

  test("a filter that would pass the limit of all the words is refused with the problem that says so", async () => {
    const full = Array.from({ length: 10 }, (_, i) => ({
      title: `Full ${String(i)}`,
      action: "warn" as const,
      contexts: ["home" as const],
      keywords: Array.from({ length: 100 }, (_, k) => ({ keyword: `w${String(i)}-${String(k)}`, whole_word: true })),
    }));
    const { data } = fakeData({ settings: { filters: full } });
    const refused = await refusedWith(saveKeywordFilter(data, request));
    expect([refused.status, refused.type]).toEqual([422, "/api/v1/problems/too-many-filter-words"]);
  });
});

describe("deleteKeywordFilter", () => {
  test("deletes the filter, and the list and the filter's own copy no longer hold it", async () => {
    const { data, requests } = fakeData();
    const [spoilers] = await listed(data);
    const id = spoilers?.id ?? "";
    await deleteKeywordFilter(data, id);
    expect(requests.filter((r) => !r.startsWith("GET "))).toEqual([`DELETE /filters/${id}`]);
    expect((await listed(data)).map((f) => f.title)).toEqual(["Election season", "Marathon week"]);
    const refused = await refusedWith(data.queryClient.query({ ...keywordFilterQuery(data, id), staleTime: 0 }));
    expect(refused.status).toBe(404);
  });

  test("a filter that is gone is refused as not found", async () => {
    const { data } = fakeData();
    expect((await refusedWith(deleteKeywordFilter(data, "gone"))).status).toBe(404);
  });
});

// ADR-0157 §10: what a filter hides is decided by the server, so after a change the posts and notifications that may
// have been filtered are read again: the screens that are open at once, the others when next shown.
describe("after a save or a delete", () => {
  /** Reads what a filter may hide into the cache, and returns the keys it covers. */
  async function cached(data: AppData, list: string, owner: string, post: string) {
    const home = homeTimelineQuery(data);
    const federated = publicTimelineQuery(data, "federated");
    const local = publicTimelineQuery(data, "local");
    const tagged = tagTimelineQuery(data, "baking");
    const listedTimeline = listTimelineQuery(data, list);
    const notifications = notificationsQuery(data, "all");
    const own = accountStatusesQuery(data, owner, "posts");
    const bookmarks = bookmarksQuery(data);
    await data.queryClient.infiniteQuery(home);
    await data.queryClient.infiniteQuery(federated);
    await data.queryClient.infiniteQuery(local);
    await data.queryClient.infiniteQuery(tagged);
    await data.queryClient.infiniteQuery(listedTimeline);
    await data.queryClient.infiniteQuery(own);
    await data.queryClient.infiniteQuery(bookmarks);
    await data.queryClient.infiniteQuery(notifications);
    const context = statusContextQuery(data, post);
    await data.queryClient.query(context);
    await data.queryClient.query(viewerQuery(data));
    return {
      marked: [
        home.queryKey,
        federated.queryKey,
        local.queryKey,
        tagged.queryKey,
        listedTimeline.queryKey,
        notifications.queryKey,
        own.queryKey,
        bookmarks.queryKey,
        context.queryKey,
      ],
      unmarked: [viewerQuery(data).queryKey],
    };
  }

  for (const how of ["save", "delete"] as const)
    test(`a ${how} marks the timelines, the notifications, the posts of an account, the contexts and the bookmarks`, async () => {
      const { fake, data } = fakeData();
      const filters = await listed(data);
      const home = await data.queryClient.infiniteQuery(homeTimelineQuery(data));
      // The newest posts of the scenario have no thread to read: take the first one whose context the server answers.
      let post = "";
      for (const status of home.pages.flatMap((p) => p.items)) {
        const read = await data.queryClient.query(statusContextQuery(data, status.id)).then(
          () => true,
          () => false,
        );
        if (read) {
          post = status.id;
          break;
        }
      }
      expect(post).not.toBe("");
      const { marked, unmarked } = await cached(
        data,
        fake.listPath(0).replace("/lists/", ""),
        fake.accountId("marco"),
        post,
      );
      if (how === "save") await saveKeywordFilter(data, request);
      else await deleteKeywordFilter(data, filters[0]?.id ?? "");
      const state = (key: readonly unknown[]) => data.queryClient.getQueryState(key);
      for (const key of marked) expect([key, state(key)?.isInvalidated]).toEqual([key, true]);
      for (const key of unmarked) expect([key, state(key)?.isInvalidated]).toEqual([key, false]);
    });

  test("the home timeline that is open is read again before the write resolves", async () => {
    const { data, requests } = fakeData();
    await data.queryClient.infiniteQuery(homeTimelineQuery(data));
    const observer = new InfiniteQueryObserver(data.queryClient, homeTimelineQuery(data));
    const unsubscribe = observer.subscribe(() => undefined);
    const reads = () => requests.filter((r) => r === "GET /timelines/home").length;
    const before = reads();
    await saveKeywordFilter(data, request);
    expect(reads() - before).toBe(1);
    unsubscribe();
  });

  test("a refused save marks nothing", async () => {
    const { data, requests } = fakeData();
    await data.queryClient.infiniteQuery(homeTimelineQuery(data));
    await refusedWith(saveKeywordFilter(data, { ...request, contexts: [] }));
    expect(data.queryClient.getQueryState(homeTimelineQuery(data).queryKey)?.isInvalidated).toBe(false);
    expect(requests.filter((r) => r === "GET /timelines/home")).toHaveLength(1);
  });
});
