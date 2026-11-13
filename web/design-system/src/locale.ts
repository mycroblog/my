// SPDX-License-Identifier: AGPL-3.0-only

// The reader's locale, and the design system's own strings in it (ADR-0009 §9, ADR-0025 §4): its catalog,
// web/design-system/locales/, compiled by @my/i18n with Lingui into messages.gen.ts (scripts/task.sh i18n:compile),
// read with Lingui. A part reads the locale and the direction from React Aria's I18nProvider, which the app places
// (the browser's language without one), as React Aria's own components do; a prop may name the locale of a part's
// words, never its direction, which is the layout's. A part that lays out its own pieces carries the direction on its
// root, so that it reads right-to-left under an RTL locale wherever it is placed. It depends on no i18n package of the
// app; a locale its catalog does not have reads en.

import { setupI18n } from "@lingui/core";
import { useLocale } from "react-aria-components";
import { type MessageId, messages } from "./messages.gen";

/** The reader's locale (`locale` when a part is given one, else the I18nProvider's) and the provider's direction. */
export function useReader(locale: string | undefined): { lang: string; direction: "ltr" | "rtl" } {
  const provided = useLocale();
  return { lang: locale ?? provided.locale, direction: provided.direction };
}

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
