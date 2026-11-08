// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import en from "virtual:locales/en/client";
import {
  applyDocumentLocale,
  createTranslate,
  direction,
  fallbackChain,
  loadI18n,
  resolveLocale,
  supportedLocales,
} from "./i18n";

describe("locales", () => {
  test("has a catalog for the source locale, the translations and the pseudo-locales", () => {
    expect(supportedLocales).toEqual(["ar-XB", "en", "en-XA", "ja", "zh-CN", "zh-TW"]);
  });

  test.each([
    ["ja-JP", ["ja-JP", "ja", "en"]],
    ["zh", ["zh", "zh-CN", "en"]],
    ["zh-CN", ["zh-CN", "en"]],
    ["zh-Hans-SG", ["zh-Hans-SG", "zh-Hans", "zh-CN", "en"]],
    ["zh-Hant-TW", ["zh-Hant-TW", "zh-Hant", "zh-TW", "en"]],
    ["zh-HK", ["zh-HK", "zh-TW", "en"]],
    ["zh-TW", ["zh-TW", "en"]],
    ["pt-PT", ["pt-PT", "pt-BR", "en"]],
    ["en", ["en"]],
    ["not a locale!", ["en"]],
  ])("falls back from %s through %j", (locale, chain) => {
    expect(fallbackChain(locale)).toEqual(chain);
  });

  test.each([
    ["ja-JP", "ja"],
    ["JA", "ja"],
    ["ar-XB", "ar-XB"],
    ["zh-CN", "zh-CN"],
    ["zh", "zh-CN"],
    ["zh-SG", "zh-CN"],
    ["zh-Hans", "zh-CN"],
    ["zh-Hans-CN", "zh-CN"],
    ["zh-TW", "zh-TW"],
    ["zh-Hant", "zh-TW"],
    ["zh-Hant-TW", "zh-TW"],
    ["zh-Hant-HK", "zh-TW"],
    ["zh-HK", "zh-TW"],
    ["zh-MO", "zh-TW"],
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

  test("loads Simplified Chinese for a tag that resolves to it", async () => {
    const { locale, messages } = await loadI18n("zh-Hans-SG");
    expect(locale).toBe("zh-CN");
    expect(messages["client:shell.nav.home"]).not.toBe(en["shell.nav.home"]?.defaultMessage);
  });

  test("loads Traditional Chinese for a tag that resolves to it", async () => {
    const { locale, messages } = await loadI18n("zh-Hant-HK");
    expect(locale).toBe("zh-TW");
    expect(messages["client:shell.nav.home"]).not.toBe(en["shell.nav.home"]?.defaultMessage);
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

test("reads a label that is not there as a neutral word", async () => {
  const t = createTranslate(await loadI18n("en"));
  const labels: Readonly<Record<string, "client:page.not_built">> = {};
  expect(t(labels.not_yet_known as "client:page.not_built")).toBe("Unknown");
});
