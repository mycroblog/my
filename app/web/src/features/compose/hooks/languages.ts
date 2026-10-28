// SPDX-License-Identifier: AGPL-3.0-only
// The languages a post can be tagged with (FR-110). The list is the languages most posts on the fediverse
// are written in, plus whatever the preference or the draft already names; the names come from the
// browser's CLDR data (Intl.DisplayNames) in the page's locale, so the catalogs carry none of them.

/** The BCP 47 tag of an undetermined language. */
const undeterminedLanguage = "und";

const commonLanguages: readonly string[] = [
  "ar",
  "ca",
  "cs",
  "da",
  "de",
  "el",
  "en",
  "eo",
  "es",
  "eu",
  "fa",
  "fi",
  "fr",
  "ga",
  "gl",
  "he",
  "hi",
  "hu",
  "id",
  "it",
  "ja",
  "ko",
  "nl",
  "no",
  "pl",
  "pt",
  "ro",
  "ru",
  "sv",
  "th",
  "tr",
  "uk",
  "vi",
  "zh",
];

export type LanguageOption = { id: string; name: string };

/** The name of `tag` in `locale`, or the tag itself when the browser has no name for it. */
function languageName(names: Intl.DisplayNames, tag: string): string {
  try {
    return names.of(tag) ?? tag;
  } catch {
    // Not a well-formed tag; show it as it is.
    return tag;
  }
}

/**
 * The language choices, sorted by name in `locale`, with "undetermined" (named `undeterminedName`) first
 * and the tags in `include` added when they are not in the list.
 */
export function languageOptions(
  locale: string,
  include: readonly string[],
  undeterminedName: string,
): LanguageOption[] {
  const names = new Intl.DisplayNames([locale], { type: "language" });
  const collator = new Intl.Collator(locale);
  const tags = [...new Set([...commonLanguages, ...include])].filter((tag) => tag !== undeterminedLanguage);
  const options = tags
    .map((id) => ({ id, name: languageName(names, id) }))
    .toSorted((a, b) => collator.compare(a.name, b.name));
  return [{ id: undeterminedLanguage, name: undeterminedName }, ...options];
}
