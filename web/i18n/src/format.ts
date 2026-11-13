// SPDX-License-Identifier: AGPL-3.0-only

// The catalog format Go and TS share (ADR-0009 §1), as a Lingui catalog-format plug-in: one locale's FormatJS JSON,
// {"<key>": {"defaultMessage": "<ICU message>", "description": "<note for translators>"}}, whose key is Lingui's
// explicit message id. The message of every locale is its translation; the source locale's is its message as well.

import type { CatalogFormatter, CatalogType } from "@lingui/conf";
import { type Entry, type Namespace, toJSON } from "@my/i18n-tools/catalog";

const isEntry = (value: unknown): value is Entry =>
  typeof value === "object" &&
  value !== null &&
  typeof (value as Entry).defaultMessage === "string" &&
  ["string", "undefined"].includes(typeof (value as Entry).description);

export const formatjs: CatalogFormatter = {
  catalogExtension: ".json",
  parse(content, { locale, sourceLocale, filename }) {
    const entries: unknown = JSON.parse(content);
    if (typeof entries !== "object" || entries === null) throw new Error(`${filename}: not a catalog`);
    const catalog: CatalogType = {};
    for (const [id, entry] of Object.entries(entries)) {
      if (!isEntry(entry)) throw new Error(`${filename} ${id}: not {"defaultMessage", "description"}`);
      catalog[id] = {
        ...(locale === sourceLocale ? { message: entry.defaultMessage } : {}),
        translation: entry.defaultMessage,
        comments: entry.description === undefined ? [] : [entry.description],
      };
    }
    return catalog;
  },
  serialize(catalog, { locale, sourceLocale }) {
    const entries: Namespace = {};
    for (const [id, { message, translation, comments = [] }] of Object.entries(catalog)) {
      const defaultMessage = translation || (locale === sourceLocale ? message : undefined);
      if (!defaultMessage) continue;
      entries[id] = { defaultMessage, ...(comments.length ? { description: comments.join("\n") } : {}) };
    }
    return toJSON(entries);
  },
};
