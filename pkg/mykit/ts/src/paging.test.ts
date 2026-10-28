// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { createClient } from "./client.js";
import { all } from "./paging.js";
import { ProblemError } from "./problem.js";
import { fakeFetch, json, problem, server, statusPage } from "./test/fake.js";

/** A home timeline of three pages: 1 2 | 3 4 | 5. */
function timeline() {
  return fakeFetch((r) => {
    const cursor = new URL(r.url).searchParams.get("cursor");
    if (cursor === null) return json(200, statusPage(["1", "2"], "c1"));
    if (cursor === "c1") return json(200, statusPage(["3", "4"], "c2"));
    if (cursor === "c2") return json(200, { ...(statusPage(["5"]) as object), next_cursor: null });
    return problem(400, "about:blank");
  });
}

describe("all", () => {
  test("follows next_cursor to the last page", async () => {
    const f = timeline();
    const api = createClient({ server, fetch: f.fetch });
    const ids: string[] = [];
    for await (const status of all(async (cursor) => {
      const { data } = await api.GET("/timelines/home", { params: { query: cursor === undefined ? {} : { cursor } } });
      if (data === undefined) throw new Error("no page");
      return data;
    })) {
      ids.push(status.id);
    }
    expect(ids).toEqual(["1", "2", "3", "4", "5"]);
    expect(f.requests).toHaveLength(3);
  });

  test("fetches no more pages when the loop breaks", async () => {
    const f = timeline();
    const api = createClient({ server, fetch: f.fetch });
    const ids: string[] = [];
    for await (const status of all(
      async (cursor) =>
        (await api.GET("/timelines/home", { params: { query: cursor === undefined ? {} : { cursor } } })).data ?? {
          items: [],
        },
    )) {
      ids.push(status.id);
      if (ids.length === 3) break;
    }
    expect(ids).toEqual(["1", "2", "3"]);
    expect(f.requests).toHaveLength(2);
  });

  test("an empty next_cursor ends it, and the first error rejects", async () => {
    const pages = all((cursor) =>
      Promise.resolve(cursor === undefined ? { items: [1], next_cursor: "" } : { items: [2] }),
    );
    const seen: number[] = [];
    for await (const n of pages) seen.push(n);
    expect(seen).toEqual([1]);

    const f = fakeFetch(() => problem(403, "/api/v1/problems/forbidden"));
    const api = createClient({ server, fetch: f.fetch });
    const failing = all(async () => (await api.GET("/timelines/home")).data ?? { items: [] });
    let error: unknown;
    try {
      for await (const status of failing) seen.push(Number(status.id));
    } catch (e) {
      error = e;
    }
    expect(error).toBeInstanceOf(ProblemError);
    expect(seen).toEqual([1]);
  });
});
