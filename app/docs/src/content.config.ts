// SPDX-License-Identifier: AGPL-3.0-only
// The content collections of the site: Starlight's docs collection, read from src/content/docs/, and its UI strings,
// read from src/content/i18n/, with the strings of the site's own components (Banner, Footer).
import { docsLoader, i18nLoader } from "@astrojs/starlight/loaders";
import { docsSchema, i18nSchema } from "@astrojs/starlight/schema";
import { defineCollection } from "astro:content";
import { z } from "astro/zod";

/** A section a translated page lists in its Banner: its English heading and its place on the English page. */
const listedSection = z.object({ heading: z.string(), href: z.string() });

export const collections = {
  docs: defineCollection({
    loader: docsLoader(),
    schema: docsSchema({
      extend: z.object({
        // Set by scripts/import-docs.ts on the pages of a locale that translate a document (src/translations.ts).
        translation: z
          .object({
            source: z.string(),
            english: z.string(),
            legal: z.boolean(),
            stale: z.array(listedSection),
            untranslated: z.array(listedSection),
          })
          .optional(),
      }),
    }),
  }),
  i18n: defineCollection({
    loader: i18nLoader(),
    schema: i18nSchema({
      extend: z
        .object({
          "docs.translation.notice": z.string(),
          "docs.translation.english": z.string(),
          "docs.translation.legal": z.string(),
          "docs.translation.stale": z.string(),
          "docs.translation.untranslated": z.string(),
          "docs.version.ended": z.string(),
          "docs.version.latest": z.string(),
          "docs.version.label": z.string(),
          "docs.llms.lead": z.string(),
          "docs.llms.page": z.string(),
        })
        .partial(),
    }),
  }),
};
