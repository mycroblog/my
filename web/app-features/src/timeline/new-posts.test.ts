// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { takesNow } from "./new-posts";

const posts = { kind: "statuses", count: 2 } as const;
const gap = { kind: "gap" } as const;

describe("whether what arrived goes in now (ADR-0013 §4, §5, §8)", () => {
  test("with the setting on, a post goes in at the top and away from it, where the feed keeps the place", () => {
    expect(takesNow(posts, { autoInsert: true, atTop: true, inside: false })).toBe(true);
    expect(takesNow(posts, { autoInsert: true, atTop: false, inside: false })).toBe(true);
  });

  test("while the reader is in the feed, it is held", () => {
    expect(takesNow(posts, { autoInsert: true, atTop: true, inside: true })).toBe(false);
    expect(takesNow(gap, { autoInsert: true, atTop: true, inside: true })).toBe(false);
  });

  test("with the setting off, everything is held until the reader presses the button", () => {
    expect(takesNow(posts, { autoInsert: false, atTop: true, inside: false })).toBe(false);
    expect(takesNow(gap, { autoInsert: false, atTop: true, inside: false })).toBe(false);
  });

  test("a break reads the first page again at the top alone; away from it, the button waits for the reader", () => {
    expect(takesNow(gap, { autoInsert: true, atTop: true, inside: false })).toBe(true);
    expect(takesNow(gap, { autoInsert: true, atTop: false, inside: false })).toBe(false);
  });
});
