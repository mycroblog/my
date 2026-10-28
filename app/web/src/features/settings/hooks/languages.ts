// SPDX-License-Identifier: AGPL-3.0-only
// The choices of the language settings, named with the browser's CLDR data (Intl.DisplayNames), so that no
// language name is written in the catalogs.
import { resolveLocale, supportedLocales } from "../../../lib/i18n";

/** Pseudo-locales are for testing; they are offered only to a user who is already displayed in one. */
const pseudoLocales: ReadonlySet<string> = new Set(["en-XA", "ar-XB"]);

/** A choice of a select or a group of checkboxes: a language tag and its name. */
export type LanguageOption = { id: string; name: string };

/** `tag`'s name in `inLocale`, or the tag itself when the browser does not know it. */
function languageName(tag: string, inLocale: string): string {
  try {
    return new Intl.DisplayNames([inLocale], { type: "language", fallback: "code" }).of(tag) ?? tag;
  } catch {
    // Not a valid tag for Intl; the tag is its own name.
    return tag;
  }
}

/**
 * The display languages: every locale the client has a catalog for, each named in its own language (as a
 * language picker does, so that a user who cannot read the current one finds theirs), sorted by tag.
 */
export function displayLanguageOptions(current: string): LanguageOption[] {
  return supportedLocales
    .filter((locale) => !pseudoLocales.has(locale) || locale === current)
    .map((locale) => ({ id: locale, name: languageName(locale, locale) }));
}

/** The display language a stored locale shows as: the supported locale closest to it (ja-JP → ja). */
export function displayLanguageOf(locale: string): string {
  return resolveLocale(locale);
}

/** Widely used languages, offered for the timeline languages besides the ones already chosen. */
const commonLanguages = [
  "ar",
  "de",
  "en",
  "es",
  "fa",
  "fr",
  "hi",
  "id",
  "it",
  "ja",
  "ko",
  "nl",
  "pl",
  "pt",
  "ru",
  "sv",
  "th",
  "tr",
  "uk",
  "vi",
  "zh",
];

/**
 * The languages a timeline can be limited to: widely used languages and the ones already chosen, named in
 * the display language and sorted by name.
 */
export function timelineLanguageOptions(chosen: readonly string[], displayLocale: string): LanguageOption[] {
  const tags = [...new Set([...commonLanguages, ...chosen])];
  const collator = new Intl.Collator(displayLocale);
  return tags
    .map((tag) => ({ id: tag, name: languageName(tag, displayLocale) }))
    .toSorted((a, b) => collator.compare(a.name, b.name));
}
