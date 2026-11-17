// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { readerLocale } from "./locale";

test("the reader's locale is the first of the browser's languages that a catalog has", () => {
  expect(readerLocale(["en-US", "ja"], "ja")).toBe("en-US");
  expect(readerLocale(["fr", "ja-JP", "en"], "en")).toBe("ja-JP");
});

test("without a browser language a catalog has, the instance's default locale, then en", () => {
  expect(readerLocale(["fr", "not a tag"], "ja")).toBe("ja");
  expect(readerLocale([], "fr")).toBe("en");
});
