// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "bun:test";
import { loadCatalog } from "../catalog";
import { createTranslator, fallbackChain } from "./translator";

const catalogs = loadCatalog();

describe("createTranslator", () => {
  test("formats en, ja and pseudo-locales through typed keys", () => {
    expect(createTranslator(catalogs, "en")("server:config.errors", { count: 1 })).toBe("Found 1 configuration error.");
    expect(createTranslator(catalogs, "ja")("server:config.errors", { count: 1 })).toBe("設定エラーが1件あります。");
    expect(createTranslator(catalogs, "en-XA")("server:errors.not_found")).toStartWith("[Ŧħḗḗ");
    expect(createTranslator(catalogs, "ar-XB")("server:errors.not_found")).toStartWith("‮");
  });

  test("falls back along the locale chain to en", () => {
    expect(createTranslator(catalogs, "ja-JP")("server:errors.not_found")).toBe("指定されたリソースが見つかりません。");
    expect(createTranslator(catalogs, "fr")("server:config.missing", { name: "MY_DATABASE" })).toBe(
      "The required setting MY_DATABASE is not set.",
    );
  });
});

describe("fallbackChain", () => {
  test("matches internal/i18n FallbackChain", () => {
    expect(fallbackChain("en")).toEqual(["en"]);
    expect(fallbackChain("ja-JP")).toEqual(["ja-JP", "ja", "en"]);
    expect(fallbackChain("zh-Hant-TW")).toEqual(["zh-Hant-TW", "zh-Hant", "zh-TW", "en"]);
    expect(fallbackChain("zh-HK")).toEqual(["zh-HK", "zh-TW", "en"]);
    expect(fallbackChain("zh-TW")).toEqual(["zh-TW", "en"]);
    expect(fallbackChain("zh-Hans-CN")).toEqual(["zh-Hans-CN", "zh-Hans", "zh-CN", "en"]);
    expect(fallbackChain("pt-PT")).toEqual(["pt-PT", "pt-BR", "en"]);
  });
});

// Type-level checks, verified by `tsc --noEmit`: wrong keys and arguments must not compile.
export function typeChecks(t: ReturnType<typeof createTranslator>) {
  t("server:errors.not_found");
  t("server:config.errors", { count: 2 });
  // @ts-expect-error unknown key
  t("server:errors.nope");
  // @ts-expect-error missing argument object
  t("server:config.errors");
  // @ts-expect-error count must be a number
  t("server:config.errors", { count: "2" });
  // @ts-expect-error unexpected argument object
  t("server:errors.not_found", { extra: 1 });
}
