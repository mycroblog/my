// SPDX-License-Identifier: AGPL-3.0-only

// The languages the settings offer for a post and for the timelines (FR-110, FR-208): the languages of ISO 639-1 that
// the platform can name, with the tags the app's own composer marks a post with beside its language's (Chinese by its
// script, Brazilian Portuguese), each by its name in its own words, in the order of the reader's language.

import { languageName } from "@my/app-components";
import type { SelectOption } from "@my/design-system";

// The two-letter codes of ISO 639-1.
const iso6391 =
  "aa ab ae af ak am an ar as av ay az ba be bg bi bm bn bo br bs ca ce ch co cr cs cu cv cy da de dv dz ee el en eo " +
  "es et eu fa ff fi fj fo fr fy ga gd gl gn gu gv ha he hi ho hr ht hu hy hz ia id ie ig ii ik io is it iu ja jv ka " +
  "kg ki kj kk kl km kn ko kr ks ku kv kw ky la lb lg li ln lo lt lu lv mg mh mi mk ml mn mr ms mt my na nb nd ne ng " +
  "nl nn no nr nv ny oc oj om or os pa pi pl ps pt qu rm rn ro ru rw sa sc sd se sg si sk sl sm sn so sq sr ss st su " +
  "sv sw ta te tg th ti tk tl tn to tr ts tt tw ty ug uk ur uz ve vi vo wa wo xh yi yo za zh zu";

/** The tags offered: ISO 639-1's, and the app's own for a script or a region of a language. */
const offered: readonly string[] = [...iso6391.split(" "), "pt-BR", "zh-CN", "zh-TW"];

/** A language by its name in its own words (English, 日本語), its first letter capitalized as a picker lists it. */
export function ownName(tag: string): string {
  const name = languageName(tag, tag) ?? tag;
  return name.charAt(0).toLocaleUpperCase(tag) + name.slice(1);
}

/** The options of each reader's language, made once: naming two hundred languages is not free. */
const made = new Map<string, readonly SelectOption[]>();

/**
 * The languages as the options of a choice, each by its own name, in the order of `lang`; a language the platform
 * has no name for is left out, and so is one it names as an earlier one: a platform whose CLDR takes Twi for Akan
 * names `tw` as `ak` does.
 */
export function languageOptions(lang: string): readonly SelectOption[] {
  let options = made.get(lang);
  if (options === undefined) {
    const collator = new Intl.Collator(lang);
    const named = new Set<string>();
    options = offered
      .map((value) => ({ value, label: ownName(value) }))
      .filter((o) => {
        if (o.label.toLowerCase() === o.value.toLowerCase() || named.has(o.label)) return false;
        named.add(o.label);
        return true;
      })
      .toSorted((a, b) => collator.compare(a.label, b.label));
    made.set(lang, options);
  }
  return options;
}

/** `options` with one for `tag` at their end, by its own name, where they have none: a value chosen elsewhere. */
export function withLanguage(options: readonly SelectOption[], tag: string): readonly SelectOption[] {
  return options.some((o) => o.value === tag) ? options : [...options, { value: tag, label: ownName(tag) }];
}
