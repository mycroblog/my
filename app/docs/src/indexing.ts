// SPDX-License-Identifier: AGPL-3.0-only
// Starlight's route middleware (astro.config.ts) for search engines: what the head of a page tells a crawler. The
// Worker serves static files only, so all of it is in the HTML of the build.
//   - The pages of a version that is not indexed (src/versions.ts: every version but /latest/, or /next/ before the
//     first release, and every page of a Preview) carry `noindex, follow` and no link to a sitemap.
//     `scripts/docs-site.sh build` points their canonical link at the same page of /latest/ when it has one.
//   - The landing page of the indexed version is the root of the domain: the build copies it to /, so its canonical
//     link, og:url and the hreflang links to it name https://mycroblog.dev/, and the sitemap link names the root
//     sitemap that `scripts/docs-site.sh build` writes.
//   - A page a locale has not translated (Starlight's fallback, the English page under /<locale>/) names the English
//     page as its canonical and claims no hreflang alternates; every page links only the alternates that are
//     translations, so no English page under /<locale>/ is announced as a translation.
import { existsSync } from "node:fs";
import { defineRouteMiddleware } from "@astrojs/starlight/route-data";
import { base, site } from "./published";
import { indexedVersion } from "./versions";

const indexed = indexedVersion();
/** The URL of the version's landing page as Starlight writes it, and the root of the domain that serves its copy. */
const landing = `${site}${base}`;
const root = `${site}/`;

/**
 * Whether the page at `url` (an hreflang alternate under the base path) is a page of the content collection, a page
 * of src/content/docs/: the pages of a locale there are its translations. Relative to the working directory, app/docs,
 * like src/page-markdown.ts.
 */
function isContentPage(url: string): boolean {
  if (!url.startsWith(landing)) {
    return false;
  }
  const id = url.slice(landing.length).replace(/\/$/, "");
  const stem = `src/content/docs/${id === "" ? "index" : id}`;
  return [".md", ".mdx", "/index.md", "/index.mdx"].some((ext) => existsSync(`${stem}${ext}`));
}

export const onRequest = defineRouteMiddleware((context) => {
  const route = context.locals.starlightRoute;
  const english = route.head.find(
    (entry) => entry.tag === "link" && entry.attrs?.["rel"] === "alternate" && entry.attrs["hreflang"] === "x-default",
  )?.attrs?.["href"];
  route.head = route.head.filter((entry) => {
    if (entry.tag !== "link" || entry.attrs?.["rel"] !== "alternate" || entry.attrs["hreflang"] === undefined) {
      return !(entry.tag === "link" && entry.attrs?.["rel"] === "sitemap" && !indexed);
    }
    if (route.isFallback === true) {
      return false;
    }
    const href = entry.attrs["href"];
    return typeof href === "string" && (href === english || isContentPage(href));
  });
  for (const entry of route.head) {
    const attrs = entry.attrs ?? {};
    const canonical =
      (entry.tag === "link" && attrs["rel"] === "canonical") ||
      (entry.tag === "meta" && attrs["property"] === "og:url");
    const key = entry.tag === "link" ? "href" : "content";
    if (canonical && route.isFallback === true && typeof english === "string") {
      attrs[key] = english;
    }
    if (indexed && attrs[key] === landing) {
      attrs[key] = root;
    }
    if (indexed && entry.tag === "link" && attrs["rel"] === "sitemap") {
      attrs["href"] = "/sitemap-index.xml";
    }
  }
  if (!indexed) {
    route.head.push({ tag: "meta", attrs: { name: "robots", content: "noindex, follow" } });
  }
});
