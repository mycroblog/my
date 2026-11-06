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
export const publishedDirs = ["operations", "reference", "guide", "developers"] as const;

/** The documents of a published directory that are for contributors, read on GitHub. */
const unpublishedFiles = new Set(["docs/reference/commands.md"]);

/** Whether a repository path (docs/operations/running.md) is a published document. */
export function isPublished(path: string): boolean {
  if (!path.endsWith(".md") || unpublishedFiles.has(path)) {
    return false;
  }
  return publishedDirs.some((dir) => path.startsWith(`docs/${dir}/`));
}

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

/** The site URL of a published document, under the base path, with a trailing slash. */
export function pageURL(path: string): string {
  return `${base}${slugOf(path)}/`;
}

export interface IndexSection {
  heading: string;
  /** The published documents the section lists, in order: their repository path and the text of the link. */
  items: { path: string; label: string }[];
}

/**
 * The sections of docs/README.md, the table of contents of the documents, in order. Each `## Heading` is a section
 * and each list item that starts with a link to a published document is one of its items; a section that lists no
 * published document (Contributing, Design) is left out.
 */
export function readIndex(): IndexSection[] {
  const source = readFileSync(`${repoRoot}docs/README.md`, "utf8");
  const sections: IndexSection[] = [];
  let current: IndexSection | undefined;
  for (const line of source.split("\n")) {
    const heading = /^## +(.+?)\s*$/.exec(line);
    if (heading?.[1] !== undefined) {
      current = { heading: heading[1], items: [] };
      sections.push(current);
      continue;
    }
    const item = /^\s*[-*] +\[([^\]]+)\]\(([^)#\s]+)/.exec(line);
    if (current === undefined || item?.[1] === undefined || item[2] === undefined) {
      continue;
    }
    const path = new URL(item[2], "file:///docs/README.md").pathname.slice(1);
    if (isPublished(path)) {
      current.items.push({ path, label: item[1] });
    }
  }
  return sections.filter((section) => section.items.length > 0);
}
