// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { screenScrollOf } from "./scroll";

const home = "/ /";

test("a screen that restores keeps one position, whichever entry of the history it shows in", () => {
  expect(screenScrollOf(home, "a", "restore")).toEqual(screenScrollOf(home, "b", "restore"));
  expect(screenScrollOf(home, "a", "top")).toEqual({ key: home, mode: "top" });
});

test("a place restores the position of the entry gone back to, and a new entry opens at the top", () => {
  const left = screenScrollOf(home, "a", "back");
  expect(left.mode).toBe("restore");
  // Back to the entry it was left at: the same key, whose position the design system brings back.
  expect(screenScrollOf(home, "a", "back")).toEqual(left);
  // Chosen again from the navigation, a new entry: a key with no position kept, which opens at the top.
  expect(screenScrollOf(home, "b", "back").key).not.toBe(left.key);
});
