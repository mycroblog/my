// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import { known } from "./known";

describe("known", () => {
  const table: Readonly<Record<"a" | "b", number>> = { a: 1, b: 2 };

  test("returns the entry of a known value", () => {
    expect(known(table, "b")).toBe(2);
  });

  test.each(["not_yet_known", "constructor", "__proto__"])("returns nothing for %s", (key) => {
    expect(known(table, key as "a")).toBeUndefined();
  });
});
