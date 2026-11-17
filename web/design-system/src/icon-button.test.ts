// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { markedSurface } from "./button";
import { compactCount } from "./icon-button";

test("the count is compact in the reader's locale", () => {
  expect(compactCount(1_234, "en")).toBe("1.2K");
  expect(compactCount(12_840, "ja")).toBe("1.3万");
  expect(compactCount(999, "en")).toBe("999");
});

test("a marked surface carries the attribute its icon buttons answer, and an unmarked one none", () => {
  expect(markedSurface(true)).toEqual({ "data-surface-marked": "" });
  expect(markedSurface(false)).toEqual({ "data-surface-marked": undefined });
});
