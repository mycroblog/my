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
    expect(supportedLocales).toEqual(["ar-XB", "de", "en", "en-XA", "es", "fr", "ja", "pt-BR", "zh-CN", "zh-TW"]);
  });

  test.each([
    ["ja-JP", ["ja-JP", "ja", "en"]],
    ["es-419", ["es-419", "es", "en"]],
    ["de-CH", ["de-CH", "de", "en"]],
    ["fr-CA", ["fr-CA", "fr", "en"]],
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
    ["pt", "pt-BR"],
    ["pt-BR", "pt-BR"],
    ["pt-PT", "pt-BR"],
    ["pt-AO", "pt-BR"],
    ["PT-br", "pt-BR"],
    ["es", "es"],
    ["es-ES", "es"],
    ["es-MX", "es"],
    ["es-419", "es"],
    ["ES-ar", "es"],
    ["de", "de"],
    ["de-DE", "de"],
    ["de-AT", "de"],
    ["de-CH", "de"],
    ["fr", "fr"],
    ["fr-FR", "fr"],
    ["fr-CA", "fr"],
    ["fr-BE", "fr"],
    ["FR-ch", "fr"],
    ["it", "en"],
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

  test("loads Spanish for a tag that resolves to it", async () => {
    const { locale, messages } = await loadI18n("es-419");
    expect(locale).toBe("es");
    expect(messages["client:shell.nav.home"]).not.toBe(en["shell.nav.home"]?.defaultMessage);
  });

  test("loads Brazilian Portuguese for a tag that resolves to it", async () => {
    const { locale, messages } = await loadI18n("pt-PT");
    expect(locale).toBe("pt-BR");
    expect(messages["client:shell.nav.home"]).not.toBe(en["shell.nav.home"]?.defaultMessage);
  });

  test("loads German and French for the tags that resolve to them", async () => {
    for (const [requested, want] of [
      ["de-AT", "de"],
      ["fr-CA", "fr"],
    ]) {
      const { locale, messages } = await loadI18n(requested as string);
      expect(locale).toBe(want);
      expect(messages["client:shell.nav.home"]).not.toBe(en["shell.nav.home"]?.defaultMessage);
    }
  });

  test("uses the source locale for an unsupported locale", async () => {
    const { locale, messages } = await loadI18n("it-IT");
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
