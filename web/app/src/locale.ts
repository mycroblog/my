// SPDX-License-Identifier: AGPL-3.0-only

// The reader's locale and the app's own strings in it (ADR-0009 §2, §9). The locale is the reader's alone: the first
// of their browser's languages that a catalog has, else the instance's default locale, else en (the locale of the
// account's preferences, first in ADR-0009 §2, is not read yet). Never the <html lang> of the server's shell, which is
// the language of the profile or the post a page previews (ADR-0028 §2). The strings are the app's catalog,
// web/app/locales/, compiled by @my/i18n with Lingui into messages.gen.ts (scripts/task.sh i18n:compile), read in the
// locale of the I18nProvider that the entry places; a locale the catalog does not have reads en.

import type { Messages } from "@lingui/core";
import { catalogReader, useLocale } from "@my/design-system";
import { type MessageId, messages } from "./messages.gen";

const stringsIn = catalogReader<MessageId>(messages satisfies Readonly<Record<string, Messages>>);

/** The app's strings in the reader's locale. */
export const useStrings = () => stringsIn(useLocale().locale).t;

/** `tag` as a canonical BCP 47 tag when a catalog has it or its language, else undefined. */
function inCatalog(tag: string): string | undefined {
  let canonical: string | undefined;
  try {
    canonical = Intl.getCanonicalLocales(tag)[0];
  } catch {
    return undefined;
  }
  if (canonical === undefined) return undefined;
  return canonical in messages || new Intl.Locale(canonical).language in messages ? canonical : undefined;
}

/**
 * The reader's locale (ADR-0009 §2): the first of `languages` (the browser's, in its order) that a catalog has, else
 * `instanceLocale`, else en.
 */
export function readerLocale(languages: readonly string[], instanceLocale: string): string {
  for (const tag of [...languages, instanceLocale]) {
    const locale = inCatalog(tag);
    if (locale !== undefined) return locale;
  }
  return "en";
}
