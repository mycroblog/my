// SPDX-License-Identifier: AGPL-3.0-only

// The design system's own strings (ADR-0009 §9, ADR-0025 §4), in en, the source, and ja, and the reader's locale its
// parts read them in: a part takes the locale as a prop and falls back to the page's language. It depends on no
// i18n package; a locale it has no strings for reads en.

const catalog = {
  en: {
    /** The button of a disclosure that shows what it folds. */
    show: "Show",
    /** The button of a disclosure that folds again what it shows. */
    hide: "Hide",
  },
  ja: {
    show: "表示",
    hide: "非表示",
  },
} as const satisfies Record<string, Record<string, string>>;

export type Strings = { readonly [K in keyof (typeof catalog)["en"]]: string };

/** The page's language, or the browser's outside a page. */
export const pageLocale = () =>
  (typeof document !== "undefined" && document.documentElement.lang) ||
  (typeof navigator !== "undefined" ? navigator.language : "en");

/** The strings of `locale` by its language subtag, and the locale they are in; en for a language without strings. */
export function stringsFor(locale: string): { strings: Strings; lang: keyof typeof catalog } {
  const lang = locale.toLowerCase().split("-")[0] === "ja" ? "ja" : "en";
  return { strings: catalog[lang], lang };
}
