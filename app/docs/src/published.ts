// SPDX-License-Identifier: AGPL-3.0-only
// Which documents of the repository the site publishes (ADR-0150 §3), where each one lands, and the settings of a
// build: the copy step (scripts/import-docs.ts) and the sidebar (src/sidebar.ts) both read this module, so they agree.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

/** The root of the repository, with a trailing slash. */
export const repoRoot = fileURLToPath(new URL("../../../", import.meta.url));

/**
 * The directories under docs/ whose Markdown is published. docs/adr, docs/architecture, docs/requirements and
 * docs/agents are never published: a directory missing here is not published, and one that does not exist yet
 * (docs/guide before its first page) is skipped.
 */
export const publishedDirs = ["operations", "reference", "guide", "developers", "plugins"] as const;

/** The documents of a published directory that are for contributors, read on GitHub. */
const unpublishedFiles = new Set(["docs/reference/commands.md"]);

/** Whether a repository path (docs/operations/running.md) is a published document. */
export function isPublished(path: string): boolean {
  if (!path.endsWith(".md") || unpublishedFiles.has(path)) {
    return false;
  }
  return publishedDirs.some((dir) => path.startsWith(`docs/${dir}/`));
}

/**
 * Files that are not Markdown documents but have a page of their own on the site (a page a plugin generates), by
 * repository path and page slug. Links to them go to that page instead of GitHub.
 */
// internal/platform/docsite, which links these pages from the server, keeps the same list.
export const sitePages: Record<string, string> = {
  // The client API reference that starlight-openapi generates (src/api-reference.ts).
  "api/openapi.yaml": "api",
};

/** The page slug of a published document: docs/operations/running.md is operations/running. */
export function slugOf(path: string): string {
  return path
    .slice("docs/".length, -".md".length)
    .toLowerCase()
    .replace(/\/readme$/, "");
}

/**
 * The base path the site is served under, set by DOCS_SITE_BASE: "/" (the default) at the root of the domain, or a
 * version prefix such as "/next/". Astro's `base` takes it as is; links written into the pages start with it.
 */
export const base = normalizeBase(process.env["DOCS_SITE_BASE"] ?? "/");

/** The origin of the site; the Markdown forms of the pages (public/<slug>.md) link with it, as they are read away from the site. */
export const site = "https://mycroblog.dev";

/** The path, relative to the site's public/ directory, of the Markdown form of the page `slug`: operations/running.md. */
export function markdownPath(slug: string): string {
  return `${slug}.md`;
}

/** The git ref the links to unpublished files point at on GitHub, set by DOCS_SITE_REF (a branch or tag; main by default). */
const ref = process.env["DOCS_SITE_REF"] || "main";

/** The GitHub URL of a file (blob) or directory (tree) of the repository at DOCS_SITE_REF. */
export function githubURL(kind: "blob" | "tree", path: string): string {
  return `https://github.com/mycroblog/my/${kind}/${ref}/${encodeURI(path)}`;
}

function normalizeBase(value: string): string {
  const trimmed = value.replace(/^\/+|\/+$/g, "");
  return trimmed === "" ? "/" : `/${trimmed}/`;
}

/**
 * The links to the pages of sitePages, with or without an #anchor, for starlight-links-validator to skip: it knows only
 * the pages of the content collection, not the ones a plugin generates.
 */
export const sitePageLinks = Object.values(sitePages).flatMap((slug) => [`${base}${slug}/`, `${base}${slug}/#*`]);

/** The site URL of a published document, under the base path, with a trailing slash. */
export function pageURL(path: string): string {
  return `${base}${slugOf(path)}/`;
}

/**
 * The site URL of the page of a repository path, for the landing page: a published document or a file of sitePages.
 * It fails the build on any other path. It also runs inside the built pages, out of reach of the repository's files:
 * starlight-links-validator checks that the hero's buttons lead to a page, and DocLinks checks its own links.
 */
export function siteLink(path: string): string {
  const sitePage = sitePages[path];
  if (sitePage !== undefined) {
    return `${base}${sitePage}/`;
  }
  if (!isPublished(path)) {
    throw new Error(`the landing page links ${path}, which is not a published document`);
  }
  return pageURL(path);
}

export interface IndexSection {
  heading: string;
  /**
   * The published documents the section lists, in order: their repository path, the text of the link, and the
   * `### Heading` of the section it is under, if any.
   */
  items: { path: string; label: string; group?: string }[];
}

/**
 * The sections of docs/README.md, the table of contents of the documents, in order. Each `## Heading` is a section,
 * each `### Heading` in it a group, and each list item that starts with a link to a published document is one of its
 * items; a section that lists no published document (Contributing, Design) is left out.
 */
export function readIndex(): IndexSection[] {
  const source = readFileSync(`${repoRoot}docs/README.md`, "utf8");
  const sections: IndexSection[] = [];
  let current: IndexSection | undefined;
  let group: string | undefined;
  for (const line of source.split("\n")) {
    const heading = /^## +(.+?)\s*$/.exec(line);
    if (heading?.[1] !== undefined) {
      current = { heading: heading[1], items: [] };
      sections.push(current);
      group = undefined;
      continue;
    }
    const subheading = /^### +(.+?)\s*$/.exec(line);
    if (subheading?.[1] !== undefined) {
      group = subheading[1];
      continue;
    }
    const item = /^\s*[-*] +\[([^\]]+)\]\(([^)#\s]+)/.exec(line);
    if (current === undefined || item?.[1] === undefined || item[2] === undefined) {
      continue;
    }
    const path = new URL(item[2], "file:///docs/README.md").pathname.slice(1);
    if (isPublished(path)) {
      current.items.push(group === undefined ? { path, label: item[1] } : { path, label: item[1], group });
    }
  }
  return sections.filter((section) => section.items.length > 0);
}
