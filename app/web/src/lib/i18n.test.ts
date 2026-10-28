// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import en from "virtual:locales/en/client";
import { applyDocumentLocale, direction, fallbackChain, loadI18n, resolveLocale, supportedLocales } from "./i18n";

describe("locales", () => {
  test("has a catalog for the source locale, Japanese and the pseudo-locales", () => {
    expect(supportedLocales).toEqual(["ar-XB", "en", "en-XA", "ja"]);
  });

  test.each([
    ["ja-JP", ["ja-JP", "ja", "en"]],
    ["zh-Hant-TW", ["zh-Hant-TW", "zh-Hant", "en"]],
    ["zh-TW", ["zh-TW", "zh-Hant", "en"]],
    ["en", ["en"]],
    ["not a locale!", ["en"]],
  ])("falls back from %s through %j", (locale, chain) => {
    expect(fallbackChain(locale)).toEqual(chain);
  });

  test.each([
    ["ja-JP", "ja"],
    ["JA", "ja"],
    ["ar-XB", "ar-XB"],
    ["fr", "en"],
    ["", "en"],
  ])("resolves %j to %s", (requested, locale) => {
    expect(resolveLocale(requested)).toBe(locale);
  });

  test.each([
    ["en", "ltr"],
    ["ja", "ltr"],
    ["en-XA", "ltr"],
    ["ar", "rtl"],
    ["ar-XB", "rtl"],
    ["he-IL", "rtl"],
    ["fa", "rtl"],
    ["az-Arab", "rtl"],
    ["uz-Latn", "ltr"],
  ])("writes %s %s", (locale, dir) => {
    expect(direction(locale)).toBe(dir);
  });
});

describe("loadI18n", () => {
  test("loads every client message of the locale", async () => {
    const { locale, messages } = await loadI18n("ja");
    expect(locale).toBe("ja");
    expect(messages["client:shell.nav.home"]).toBe("ホーム");
    expect(Object.keys(messages).toSorted()).toEqual(
      Object.keys(en)
        .map((key) => `client:${key}`)
        .toSorted(),
    );
  });

  test("uses the source locale for an unsupported locale", async () => {
    const { locale, messages } = await loadI18n("fr-FR");
    expect(locale).toBe("en");
    expect(messages["client:shell.nav.home"]).toBe(en["shell.nav.home"]?.defaultMessage);
  });
});

test("applyDocumentLocale sets lang and dir of <html>", () => {
  const doc = document.implementation.createHTMLDocument();
  applyDocumentLocale(doc, "ar-XB");
  expect(doc.documentElement.lang).toBe("ar-XB");
  expect(doc.documentElement.dir).toBe("rtl");
});
