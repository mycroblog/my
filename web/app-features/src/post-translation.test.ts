// SPDX-License-Identifier: AGPL-3.0-only

// Whether a post is in another language than the reader's. What a drawn post offers and where Translate goes is in
// post-translation.dom.test.tsx.

import { expect, test } from "bun:test";
import { inAnotherLanguage } from "./post-translation";

test("a post is in another language than the reader's: another language, or the same in another script", () => {
  expect(inAnotherLanguage("ja", "en")).toBe(true);
  expect(inAnotherLanguage("en", "ja")).toBe(true);
  expect(inAnotherLanguage("en", "en")).toBe(false);
  expect(inAnotherLanguage("en-GB", "en")).toBe(false);
  expect(inAnotherLanguage("en", "en-US")).toBe(false);
  expect(inAnotherLanguage("zh-Hant", "zh-Hans")).toBe(true);
  expect(inAnotherLanguage("zh-Hant-TW", "zh-Hant")).toBe(false);
});

test("a post whose language is not known is not offered: it may be the reader's", () => {
  expect(inAnotherLanguage("und", "en")).toBe(false);
  expect(inAnotherLanguage("", "en")).toBe(false);
});
