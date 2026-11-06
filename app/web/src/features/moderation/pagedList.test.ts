// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from "vitest";
import { wholeList, wholeListPageSize } from "./hooks/pagedList";

describe("wholeList", () => {
  it("pages a whole list on the client, 40 rows a page, with nothing more to load", () => {
    const items = Array.from({ length: 1000 }, (_, index) => index);
    const list = wholeList(items);
    expect(wholeListPageSize).toBe(40);
    expect(list.items).toHaveLength(1000);
    expect(list.pages).toHaveLength(25);
    expect(list.pages[0]).toEqual(items.slice(0, 40));
    expect(list.pages.at(-1)).toEqual(items.slice(960));
    expect(list.hasMore).toBe(false);
  });

  it("keeps a last page shorter than the others, and no page for an empty list", () => {
    expect(wholeList([1, 2, 3], 2).pages).toEqual([[1, 2], [3]]);
    expect(wholeList([]).pages).toEqual([]);
  });
});
