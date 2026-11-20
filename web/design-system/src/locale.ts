// SPDX-License-Identifier: AGPL-3.0-only

// The reader's locale, and the design system's own strings in it (ADR-0009 §9, ADR-0025 §4): its catalog,
// web/design-system/locales/, compiled by @my/i18n with Lingui into messages.gen.ts (scripts/task.sh i18n:compile),
// read with Lingui. A part reads the locale and the direction from React Aria's I18nProvider, which the app places
// (the browser's language without one), as React Aria's own components do; a prop may name the locale of a part's
// words, never its direction, which is the layout's. A part that lays out its own pieces carries the direction on its
// root, so that it reads right-to-left under an RTL locale wherever it is placed. It depends on no i18n package of the
// app; a locale its catalog does not have reads en, and so does one whose messages the page has not loaded yet.

import { setupI18n, type Messages } from "@lingui/core";
import { useLocale } from "react-aria-components";
import { catalog, type MessageId } from "./messages.gen";

/** The reader's locale (`locale` when a part is given one, else the I18nProvider's) and the provider's direction. */
export function useReader(locale: string | undefined): { lang: string; direction: "ltr" | "rtl" } {
  const provided = useLocale();
  return { lang: locale ?? provided.locale, direction: provided.direction };
}

/** The locale of a catalog of `locales` that `locale` resolves to (ADR-0009 §2): the tag, else its language; en for neither. */
function catalogLocale(locale: string, locales: ReadonlySet<string>): string {
  let tag: string | undefined;
  try {
    tag = Intl.getCanonicalLocales(locale)[0];
  } catch {
    return "en";
  }
  if (tag === undefined) return "en";
  if (locales.has(tag)) return tag;
  const { language } = new Intl.Locale(tag);
  return locales.has(language) ? language : "en";
}

/** A package's strings in one locale: each by its id, with the values of its arguments, and the locale they are in. */
export type Strings<Id extends string> = {
  t: (id: Id, values?: Readonly<Record<string, string | number>>) => string;
  lang: string;
};

/** A package's compiled catalog (its messages.gen.ts): the en messages, which every locale falls back to, and its locales. */
export type Catalog = { readonly source: Messages; readonly locales: readonly string[] };

/** What loads a locale's messages on demand, by the locale (a package's messages-load.gen.ts). */
export type Loaders = Readonly<Record<string, () => Promise<Messages>>>;

/**
 * The reader of a package's catalog: its strings in a locale, resolved as the design system's own are, in en while the
 * messages of the locale are not read yet. A page reads the en messages with the catalog, and the others as `load` or
 * `add` gives them, before it shows a locale (ADR-0009 §1).
 */
export type CatalogReader<Id extends string> = ((locale: string) => Strings<Id>) & {
  /** Whether the messages of the locale `locale` resolves to are read. */
  has: (locale: string) => boolean;
  /** Reads these messages, by their locales, from now on. */
  add: (messages: Readonly<Record<string, Messages>>) => void;
  /** Loads with `loaders` the messages of the locale `locale` resolves to, unless they are read already. */
  load: (locale: string, loaders: Loaders) => Promise<void>;
};

/**
 * The reader of a package's compiled catalog: the package's strings in a locale, resolved as the design system's own
 * are, so that a package above it reads its own catalog the same way.
 */
export function catalogReader<Id extends string>(catalog: Catalog): CatalogReader<Id> {
  const locales = new Set(catalog.locales);
  const readers = new Map([["en", setupI18n({ locale: "en", messages: { en: catalog.source } })]]);
  const loading = new Map<string, Promise<void>>();
  const add = (messages: Readonly<Record<string, Messages>>) => {
    for (const [locale, m] of Object.entries(messages))
      if (!readers.has(locale)) readers.set(locale, setupI18n({ locale, messages: { [locale]: m } }));
  };
  const read = (locale: string): Strings<Id> => {
    const lang = catalogLocale(locale, locales);
    const i18n = readers.get(lang);
    if (!i18n) return read("en");
    return { t: (id, values) => i18n._(id, values), lang };
  };
  return Object.assign(read, {
    has: (locale: string) => readers.has(catalogLocale(locale, locales)),
    add,
    load(locale: string, loaders: Loaders): Promise<void> {
      const lang = catalogLocale(locale, locales);
      const loader = loaders[lang];
      if (readers.has(lang) || loader === undefined) return Promise.resolve();
      let pending = loading.get(lang);
      if (pending === undefined) {
        pending = loader().then(
          (messages) => {
            add({ [lang]: messages });
          },
          (error: unknown) => {
            loading.delete(lang);
            throw error;
          },
        );
        loading.set(lang, pending);
      }
      return pending;
    },
  });
}

/** The design system's strings in `locale`, by their ids, and the locale they are in. */
export const stringsIn = catalogReader<MessageId>(catalog);
