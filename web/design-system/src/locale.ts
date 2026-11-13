// SPDX-License-Identifier: AGPL-3.0-only

// The reader's locale, and the design system's own strings in it (ADR-0009 §9, ADR-0025 §4): its catalog,
// web/design-system/locales/, compiled by @my/i18n with Lingui into messages.gen.ts (scripts/task.sh i18n:compile),
// read with Lingui. A part takes the locale as a prop and falls back to the page's language. It depends on no i18n
// package of the app; a locale its catalog does not have reads en.

import { setupI18n } from "@lingui/core";
import { type MessageId, messages } from "./messages.gen";

/** The page's language, or the browser's outside a page. */
export const pageLocale = () =>
  (typeof document !== "undefined" && document.documentElement.lang) ||
  (typeof navigator !== "undefined" ? navigator.language : "en");

/** The locale of the catalog that `locale` resolves to (ADR-0009 §2): the tag, else its language; en for neither. */
function catalogLocale(locale: string): string {
  let tag: string | undefined;
  try {
    tag = Intl.getCanonicalLocales(locale)[0];
  } catch {
    return "en";
  }
  if (tag === undefined) return "en";
  if (tag in messages) return tag;
  const { language } = new Intl.Locale(tag);
  return language in messages ? language : "en";
}

const readers = new Map(
  Object.entries(messages).map(([locale, m]) => [locale, setupI18n({ locale, messages: { [locale]: m } })]),
);

/** The design system's strings in `locale`, by their ids, and the locale they are in. */
export function stringsIn(locale: string): { t: (id: MessageId) => string; lang: string } {
  const lang = catalogLocale(locale);
  const i18n = readers.get(lang);
  if (!i18n) throw new Error(`the design system's catalog has no ${lang}`);
  return { t: (id) => i18n._(id), lang };
}
