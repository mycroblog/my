// SPDX-License-Identifier: AGPL-3.0-only
// The Markdown form of a page (scripts/import-docs.ts writes it to public/<slug>.md): its URL, which the route
// middleware (src/route-middleware.ts) links from the head of the page and the footer from its bottom. The pages
// without one (the landing page, the API reference) get neither.
import { existsSync } from "node:fs";
import { base, markdownPath } from "./published";

// Relative to the working directory, which is the project root (app/docs) for every `astro` command: this module is
// bundled for the build, so import.meta.url does not locate the project.
/** The URL of the Markdown form of the page with the Starlight id `id` (operations/running), or undefined if it has none. */
export function markdownURL(id: string): string | undefined {
  const path = markdownPath(id);
  return id !== "" && existsSync(`public/${path}`) ? `${base}${path}` : undefined;
}
