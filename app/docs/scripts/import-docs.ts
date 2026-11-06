// SPDX-License-Identifier: AGPL-3.0-only
// Copies the published documents of the repository (src/published.ts) into src/content/docs/ before a build or the
// dev server (ADR-0150 §4): `bun run import` from package.json's build and dev scripts. The copies are generated and
// git-ignored; the hand-written landing page src/content/docs/index.md stays. Each copy differs from its source in:
//   - the first `# Heading`, which becomes the `title` of the frontmatter (Starlight renders it as the page heading);
//   - relative links to published documents, which become site URLs under the base path, #anchors kept;
//   - relative links to a file with a page of its own (sitePages), which become that page;
//   - relative links to any other file of the repository, which become GitHub URLs at DOCS_SITE_REF.
// The sources stay readable on GitHub as they are; `agentkit docs lint` checks their links and anchors, and the build
// checks the links of the pages (starlight-links-validator).
import { existsSync, mkdirSync, readdirSync, rmSync, statSync, writeFileSync, readFileSync } from "node:fs";
import { dirname, join, posix } from "node:path";
import { base, githubURL, isPublished, pageURL, publishedDirs, repoRoot, sitePages, slugOf } from "../src/published";

const contentDir = join(repoRoot, "app/docs/src/content/docs");
const handWritten = new Set(["index.md"]);

function markdownFiles(dir: string): string[] {
  const absolute = join(repoRoot, dir);
  if (!existsSync(absolute)) {
    return [];
  }
  return readdirSync(absolute, { recursive: true, encoding: "utf8" })
    .map((name) => posix.join(dir, name.split("\\").join("/")))
    .filter((path) => path.endsWith(".md") && statSync(join(repoRoot, path)).isFile())
    .sort();
}

/** The new destination of a link written in the document at `from`, or undefined to leave it as it is. */
function rewrite(from: string, dest: string): string | undefined {
  if (dest === "" || dest.startsWith("#") || dest.startsWith("//") || /^[A-Za-z][A-Za-z0-9+.-]*:/.test(dest)) {
    return undefined;
  }
  const hash = dest.indexOf("#");
  const file = hash === -1 ? dest : dest.slice(0, hash);
  const anchor = hash === -1 ? "" : dest.slice(hash);
  const target = posix.normalize(posix.join(posix.dirname(from), decodeURIComponent(file.split("?")[0] ?? "")));
  if (target.startsWith("../")) {
    throw new Error(`${from}: the link ${dest} leaves the repository`);
  }
  const sitePage = sitePages[target];
  if (sitePage !== undefined) {
    return `${base}${sitePage}/${anchor}`;
  }
  if (isPublished(target)) {
    return pageURL(target) + anchor;
  }
  const absolute = join(repoRoot, target);
  if (!existsSync(absolute)) {
    throw new Error(`${from}: the link ${dest} names ${target}, which does not exist`);
  }
  const kind = statSync(absolute).isDirectory() ? "tree" : "blob";
  return githubURL(kind, target.replace(/\/$/, "")) + anchor;
}

const reInlineLink = /(!?\[(?:[^\]\\]|\\.)*\]\()(<[^>]*>|[^)\s]+)((?:\s+"[^"]*")?\))/g;
const reRefDefinition = /^(\s{0,3}\[[^\]]+\]:\s*)(<[^>]*>|\S+)(.*)$/;

/** Rewrites the links of one line outside code spans. */
function rewriteLine(from: string, line: string): string {
  const definition = reRefDefinition.exec(line);
  if (definition?.[1] !== undefined && definition[2] !== undefined) {
    const dest = definition[2].replace(/^<|>$/g, "");
    const next = rewrite(from, dest);
    return next === undefined ? line : `${definition[1]}${next}${definition[3] ?? ""}`;
  }
  // A link that starts inside a code span (`[x](y)`) is code, not a link; a code span inside the text of a link is not.
  const code = [...line.matchAll(/(`+)[^`]*?\1/g)].map((m) => [m.index, m.index + m[0].length] as const);
  return line.replace(reInlineLink, (whole: string, open: string, dest: string, close: string, at: number) => {
    if (code.some(([start, end]) => at >= start && at < end)) {
      return whole;
    }
    const next = rewrite(from, dest.replace(/^<|>$/g, ""));
    return next === undefined ? whole : `${open}${next}${close}`;
  });
}

/** The page for the document at `path`: frontmatter with its title, then its body with the links rewritten. */
function transform(path: string, source: string): string {
  const lines = source.split("\n");
  const titleAt = lines.findIndex((line) => /^# +\S/.test(line));
  if (titleAt === -1) {
    throw new Error(`${path}: no "# Heading" to take the title from`);
  }
  const title = (lines[titleAt] ?? "").replace(/^# +/, "").trim();
  lines.splice(titleAt, 1);
  let fence = "";
  const body = lines.map((line) => {
    const marker = /^\s{0,3}(`{3,}|~{3,})/.exec(line)?.[1];
    if (marker !== undefined && (fence === "" || (marker[0] === fence[0] && marker.length >= fence.length))) {
      fence = fence === "" ? marker : "";
      return line;
    }
    return fence === "" ? rewriteLine(path, line) : line;
  });
  const frontmatter = [
    "---",
    `title: ${JSON.stringify(title)}`,
    `# Generated from ${path} by app/docs/scripts/import-docs.ts.`,
    "---",
  ];
  return [...frontmatter, ...body].join("\n").replace(/\n*$/, "\n");
}

for (const name of readdirSync(contentDir)) {
  if (!handWritten.has(name)) {
    rmSync(join(contentDir, name), { recursive: true, force: true });
  }
}
const published = publishedDirs.flatMap((dir) => markdownFiles(`docs/${dir}`)).filter(isPublished);
for (const path of published) {
  const out = join(contentDir, `${slugOf(path)}.md`);
  mkdirSync(dirname(out), { recursive: true });
  writeFileSync(out, transform(path, readFileSync(join(repoRoot, path), "utf8")));
}
console.log(`import-docs: copied ${published.length} documents into src/content/docs/`);
