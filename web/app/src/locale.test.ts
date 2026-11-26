// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { appLanguages, readerLocale } from "./locale";

test("the reader's locale is the first of the browser's languages that a catalog has", () => {
  expect(readerLocale(["en-US", "ja"], "ja")).toBe("en-US");
  expect(readerLocale(["it", "ja-JP", "en"], "en")).toBe("ja-JP");
});

test("the languages a reader may choose are those the app has words in, without the pseudo-locales of its tests", () => {
  expect(appLanguages).toEqual(["de", "en", "es", "fr", "ja", "pt-BR", "zh-CN", "zh-TW"]);
  // Each of them is one a reader's browser may ask for.
  for (const language of appLanguages) expect(readerLocale([language], "en")).toBe(language);
});

test("without a browser language a catalog has, the instance's default locale, then en", () => {
  expect(readerLocale(["it", "not a tag"], "ja")).toBe("ja");
  expect(readerLocale([], "it")).toBe("en");
});
