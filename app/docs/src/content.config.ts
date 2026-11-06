// SPDX-License-Identifier: AGPL-3.0-only
// The content collections of the site: Starlight's docs collection, read from src/content/docs/.
import { docsLoader } from "@astrojs/starlight/loaders";
import { docsSchema } from "@astrojs/starlight/schema";
import { defineCollection } from "astro:content";

export const collections = {
  docs: defineCollection({ loader: docsLoader(), schema: docsSchema() }),
};
