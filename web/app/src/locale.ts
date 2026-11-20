// SPDX-License-Identifier: AGPL-3.0-only

// The reader's locale and the app's own strings in it (ADR-0009 §2, §9). The locale is the reader's alone: the
// language their account chose, when it chose one and a catalog has it (reader.tsx), else the first of their browser's
// languages that a catalog has, else the instance's default locale, else en. Never the <html lang> of the server's
// shell, which is the language of the profile or the post a page previews (ADR-0028 §2). The strings are the app's
// catalog, web/app/locales/, compiled by @my/i18n with Lingui into messages.gen.ts (scripts/task.sh i18n:compile), read
// in the locale of the I18nProvider that the app places; a locale the catalog does not have reads en.

import type { Messages } from "@lingui/core";
import { catalogReader, useLocale } from "@my/design-system";
import { catalog, type MessageId } from "./messages.gen";

/** The app's strings in `locale`, by their ids, and the locale they are in. */
export const stringsIn = catalogReader<MessageId>(catalog satisfies { readonly source: Messages });

/** The app's strings in the reader's locale. */
export const useStrings = () => stringsIn(useLocale().locale).t;

/**
 * The languages the app can be shown in, which the settings offer: the locales of the catalog, without the
 * pseudo-locales made from en (ADR-0009 §1), which carry a private-use region.
 */
export const appLanguages: readonly string[] = catalog.locales.filter((tag) => !/-X[A-Z]$/.test(tag)).toSorted();

/** `tag` as a canonical BCP 47 tag when a catalog has it or its language, else undefined. */
export function inCatalog(tag: string): string | undefined {
  let canonical: string | undefined;
  try {
    canonical = Intl.getCanonicalLocales(tag)[0];
  } catch {
    return undefined;
  }
  if (canonical === undefined) return undefined;
  const locales: readonly string[] = catalog.locales;
  return locales.includes(canonical) || locales.includes(new Intl.Locale(canonical).language) ? canonical : undefined;
}

/**
 * The locale of a reader whose account chose none (ADR-0009 §2): the first of `languages` (the browser's, in its
 * order) that a catalog has, else `instanceLocale`, else en.
 */
export function readerLocale(languages: readonly string[], instanceLocale: string): string {
  for (const tag of [...languages, instanceLocale]) {
    const locale = inCatalog(tag);
    if (locale !== undefined) return locale;
  }
  return "en";
}
