// SPDX-License-Identifier: AGPL-3.0-only
// Typed translator over FormatJS intl-messageformat, the TypeScript counterpart of
// internal/i18n. Keys and argument types come from the generated MessageArgs, so an
// unknown key or a wrong argument is a compile-time error (see types.test.ts).
import { IntlMessageFormat } from "intl-messageformat";
import type { MessageArgs, MessageKey } from "./messages.gen";

/** locale -> namespace -> key -> { defaultMessage } (the FormatJS JSON catalog shape). */
export type Catalogs = Record<string, Record<string, Record<string, { defaultMessage: string }>>>;

const SOURCE = "en";
const PSEUDO = new Set(["en-XA", "ar-XB"]);

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
    if (l in PARENTS) {
      l = PARENTS[l];
      continue;
    }
    const i = l.lastIndexOf("-");
    if (i < 0) break;
    l = l.slice(0, i);
  }
  if (!chain.includes(SOURCE)) chain.push(SOURCE);
  return chain;
}

/** Pseudo-locales are formatted with the source locale's CLDR rules. */
export const formattingLocale = (locale: string): string => (PSEUDO.has(locale) ? SOURCE : locale);

type ArgsParam<K extends MessageKey> = keyof MessageArgs[K] extends never ? [] : [args: MessageArgs[K]];

export type Translate = <K extends MessageKey>(key: K, ...args: ArgsParam<K>) => string;

export function createTranslator(catalogs: Catalogs, locale: string): Translate {
  const chain = fallbackChain(locale);
  const cache = new Map<string, IntlMessageFormat>();
  return (key, ...[args]) => {
    const [ns, id] = key.split(/:(.*)/s);
    for (const loc of chain) {
      const source = catalogs[loc]?.[ns]?.[id]?.defaultMessage;
      if (source === undefined) continue;
      const cacheKey = `${loc}\u0000${key}`;
      let mf = cache.get(cacheKey);
      if (!mf) {
        mf = new IntlMessageFormat(source, formattingLocale(loc));
        cache.set(cacheKey, mf);
      }
      return String(mf.format(args as Parameters<IntlMessageFormat["format"]>[0]));
    }
    return key;
  };
}
