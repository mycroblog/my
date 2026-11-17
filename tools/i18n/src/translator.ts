// SPDX-License-Identifier: AGPL-3.0-only
// Formats catalog messages with FormatJS intl-messageformat the way internal/i18n formats them, for the parity
// golden file (generate.ts).
import { type Formatters, IntlMessageFormat } from "intl-messageformat";
import { type Catalog, formattingLocale, SOURCE } from "./catalog";

/**
 * Same overrides as parentLocales in internal/i18n (ADR-0009 §2): each Chinese and Portuguese tag reaches the one
 * catalog locale of its language and script, and a catalog locale with a region ends its chain before en.
 */
const PARENTS: Record<string, string> = {
  zh: "zh-CN",
  "zh-SG": "zh-CN",
  "zh-Hans": "zh-CN",
  "zh-CN": "",
  "zh-Hant": "zh-TW",
  "zh-HK": "zh-TW",
  "zh-MO": "zh-TW",
  "zh-TW": "",
  pt: "pt-BR",
  "pt-PT": "pt-BR",
  "pt-BR": "",
};

/** zh-Hant-TW -> zh-Hant -> zh-TW -> en, ja-JP -> ja -> en (ADR-0009 §2). */
export function fallbackChain(locale: string): string[] {
  const chain: string[] = [];
  for (let l = locale; l !== "";) {
    chain.push(l);
    const parent = PARENTS[l];
    if (parent !== undefined) {
      l = parent;
      continue;
    }
    const i = l.lastIndexOf("-");
    if (i < 0) break;
    l = l.slice(0, i);
  }
  if (!chain.includes(SOURCE)) chain.push(SOURCE);
  return chain;
}

/** One formatter per constructor and arguments, as FormatJS's default formatters keep them. */
function memoized<A extends unknown[], T>(make: (...args: A) => T): (...args: A) => T {
  const cache = new Map<string, T>();
  return (...args) => {
    const key = JSON.stringify(args);
    let value = cache.get(key);
    if (value === undefined) {
      value = make(...args);
      cache.set(key, value);
    }
    return value;
  };
}

/**
 * The formatters of every message. Numbers group their digits from four digits on in every locale (useGrouping
 * "always"), as internal/i18n formats them: its engine does not apply CLDR's minimumGroupingDigits, which keeps
 * 1234 ungrouped in es, pl and pt-PT (ADR-0009 §1).
 */
const formatters: Formatters = {
  getNumberFormat: memoized(
    (locales?: string | string[], opts?: Intl.NumberFormatOptions) =>
      new Intl.NumberFormat(locales, { useGrouping: "always", ...opts }),
  ),
  getDateTimeFormat: memoized(
    (...args: ConstructorParameters<typeof Intl.DateTimeFormat>) => new Intl.DateTimeFormat(...args),
  ),
  getPluralRules: memoized((...args: ConstructorParameters<typeof Intl.PluralRules>) => new Intl.PluralRules(...args)),
};

/** Compiles message for locale with the formatters that match internal/i18n. */
export const compileMessage = (message: string, locale: string): IntlMessageFormat =>
  new IntlMessageFormat(message, formattingLocale(locale), undefined, { formatters });

export type Translate = (id: string, args?: Record<string, string | number>) => string;

/** Formats "<namespace>:<key>" in the first locale of the fallback chain that has it; the id itself when none has. */
export function createTranslator(catalog: Catalog, locale: string): Translate {
  const chain = fallbackChain(locale);
  const cache = new Map<string, IntlMessageFormat>();
  return (id, args) => {
    const [ns = "", key = ""] = id.split(/:(.*)/s);
    for (const loc of chain) {
      const source = catalog[loc]?.[ns]?.[key]?.defaultMessage;
      if (source === undefined) continue;
      const cacheKey = `${loc}\u0000${id}`;
      let mf = cache.get(cacheKey);
      if (!mf) {
        mf = compileMessage(source, loc);
        cache.set(cacheKey, mf);
      }
      return String(mf.format(args));
    }
    return id;
  };
}
