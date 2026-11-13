// SPDX-License-Identifier: AGPL-3.0-only

// The reader's locale, and the design system's own strings in it (ADR-0009 §9, ADR-0025 §4): its catalog,
// web/design-system/locales/, compiled by @my/i18n with Lingui into messages.gen.ts (scripts/task.sh i18n:compile),
// read with Lingui. A part reads the locale and the direction from React Aria's I18nProvider, which the app places
// (the browser's language without one), as React Aria's own components do; a prop may name the locale of a part's
// words, never its direction, which is the layout's. A part that lays out its own pieces carries the direction on its
// root, so that it reads right-to-left under an RTL locale wherever it is placed. It depends on no i18n package of the
// app; a locale its catalog does not have reads en.

import { setupI18n, type Messages } from "@lingui/core";
import { useLocale } from "react-aria-components";
import { type MessageId, messages } from "./messages.gen";

/** The reader's locale (`locale` when a part is given one, else the I18nProvider's) and the provider's direction. */
export function useReader(locale: string | undefined): { lang: string; direction: "ltr" | "rtl" } {
  const provided = useLocale();
  return { lang: locale ?? provided.locale, direction: provided.direction };
}

/** The locale of the catalog of `messages` that `locale` resolves to (ADR-0009 §2): the tag, else its language; en for neither. */
function catalogLocale(locale: string, messages: Readonly<Record<string, Messages>>): string {
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

/** A package's strings in one locale: each by its id, with the values of its arguments, and the locale they are in. */
export type Strings<Id extends string> = {
  t: (id: Id, values?: Readonly<Record<string, string | number>>) => string;
  lang: string;
};

/**
 * The reader of a package's compiled catalog (its messages.gen.ts): the package's strings in a locale, resolved as the
 * design system's own are, so that a package above it reads its own catalog the same way.
 */
export function catalogReader<Id extends string>(
  messages: Readonly<Record<string, Messages>>,
): (locale: string) => Strings<Id> {
  const readers = new Map(
    Object.entries(messages).map(([locale, m]) => [locale, setupI18n({ locale, messages: { [locale]: m } })]),
  );
  return (locale) => {
    const lang = catalogLocale(locale, messages);
    const i18n = readers.get(lang);
    if (!i18n) throw new Error(`the catalog has no ${lang}`);
    return { t: (id, values) => i18n._(id, values), lang };
  };
}

/** The design system's strings in `locale`, by their ids, and the locale they are in. */
export const stringsIn = catalogReader<MessageId>(messages);
