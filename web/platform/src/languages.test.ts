// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { browserLanguages } from "./languages";

const original = Object.getOwnPropertyDescriptor(globalThis, "navigator");

const navigatorWith = (value: unknown) => {
  Object.defineProperty(globalThis, "navigator", { value, configurable: true });
};

afterEach(() => {
  if (original) Object.defineProperty(globalThis, "navigator", original);
});

test("the browser's languages in the reader's order, and none without a navigator", () => {
  navigatorWith({ languages: ["ja-JP", "en"] });
  expect(browserLanguages()).toEqual(["ja-JP", "en"]);
  navigatorWith(undefined);
  expect(browserLanguages()).toEqual([]);
});
