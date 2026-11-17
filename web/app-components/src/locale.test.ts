// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { languageName } from "./locale";

test("Chinese is named by its script in every language, never by a region", () => {
  expect(languageName("zh-CN", "zh-CN")).toBe("简体中文");
  expect(languageName("zh-TW", "zh-TW")).toBe("繁體中文");
  expect(languageName("zh-HK", "zh-TW")).toBe("繁體中文");
  expect(languageName("zh-SG", "en")).toBe("Simplified Chinese");
  expect(languageName("zh-Hant-TW", "en")).toBe("Traditional Chinese");
  expect(languageName("zh-TW", "ja")).toBe("繁体中国語");
  expect(languageName("zh", "en")).toBe("Chinese");
});

test("another language by its name, and none for a malformed tag", () => {
  expect(languageName("pt-BR", "en")).toBe("Brazilian Portuguese");
  expect(languageName("ja", "ja")).toBe("日本語");
  expect(languageName("not a tag", "en")).toBeUndefined();
});
