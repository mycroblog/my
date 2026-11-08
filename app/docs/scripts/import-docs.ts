// SPDX-License-Identifier: AGPL-3.0-only
// Copies the published documents of the repository (src/published.ts) into src/content/docs/ before a build or the
// dev server (ADR-0150 §4): `bun run import` from package.json's build and dev scripts. The copies are generated and
// git-ignored; the pages written in the site (the landing page src/content/docs/index.mdx and the analytics notice
// src/content/docs/analytics.mdx) stay. Each copy differs from its source in:
//   - the first `# Heading`, which becomes the `title` of the frontmatter (Starlight renders it as the page heading);
//   - relative links to published documents, which become site URLs under the base path, #anchors kept;
//   - relative links to a file with a page of its own (sitePages), which become that page;
//   - relative links to an image of a published directory (docs/operations/images/architecture.svg), which is copied
//     to the same place beside the page so that Astro serves it, and stay relative (GitHub URLs in the plain Markdown);
//   - relative links to any other file of the repository, which become GitHub URLs at DOCS_SITE_REF.
// Each page is also written, with the same rewritten links but absolute (https://mycroblog.dev/next/...), as plain
// Markdown to public/<slug>.md, which the build serves at the URL of the page without its trailing slash plus .md (the
// convention of https://llmstxt.org/). The generated files under public/ are git-ignored.
// The sources stay readable on GitHub as they are; `agentkit docs lint` checks their links and anchors, and the build
// checks the links of the pages (starlight-links-validator).
//
// The translations of app/docs/i18n/<locale>/ (src/translations.ts, ADR-0150 §9) become the pages of that locale,
// src/content/docs/<locale>/: each section of the source in its order, translated where the translation has it and in
// English where it does not, every heading with the anchor of its English heading ({#id}), the links rewritten as if
// written in the source (to the pages of the locale), and the frontmatter's `translation` naming the English page and
// the sections that are stale or untranslated, which the Banner lists. A page with no translation file gets none, and
// Starlight serves the English page in its place with a notice. The translations have no plain Markdown form.
import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync,
  readFileSync,
} from "node:fs";
import { dirname, join, posix } from "node:path";
import {
  base,
  githubURL,
  isPublished,
  markdownPath,
  pageURL,
  publishedDirs,
  repoRoot,
  site,
  sitePages,
  slugOf,
} from "../src/published";
import {
  filesIn,
  headingOf,
  handWrittenPages,
  isHandWritten,
  landingPage,
  legalTemplates,
  localeSegment,
  type PageTranslation,
  translationLocales,
  translationOf,
  translatableSources,
} from "../src/translations";

const contentDir = join(repoRoot, "app/docs/src/content/docs");
const publicDir = join(repoRoot, "app/docs/public");
const handWritten = new Set(handWrittenPages.map((path) => posix.basename(path)));

/** What a page is written as: a page of the site (in a locale, for a translation) or the plain Markdown form. */
interface Output {
  absolute: boolean;
  /** The directory of the locale in the URL space (localeSegment), for a page of a locale. */
  locale?: string;
}

/** The images a published document may show, copied beside the pages. */
const reImage = /\.(svg|png|jpe?g|webp)$/;

/** Whether a repository path is an image of a published directory. */
function isPublishedImage(path: string): boolean {
  return reImage.test(path) && publishedDirs.some((dir) => path.startsWith(`docs/${dir}/`));
}

/** The new destination of a link written in the document at `from`, or undefined to leave it as it is. */
function rewrite(from: string, dest: string, out: Output): string | undefined {
  const next = rewriteToSite(from, dest, out);
  return next !== undefined && out.absolute && next.startsWith("/") ? site + next : next;
}

function rewriteToSite(from: string, dest: string, out: Output): string | undefined {
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
    return pageURL(target, out.locale) + anchor;
  }
  if (isPublishedImage(target) && existsSync(join(repoRoot, target))) {
    if (out.absolute) {
      // The plain Markdown under public/ has no copy of the image beside it.
      return githubURL("blob", target) + "?raw=true";
    }
    // Astro resolves an image of a page only from a path that starts with ./ or ../. The page of a locale sits one
    // directory deeper (src/content/docs/ja/operations/), the image where the English page has it.
    const dir =
      out.locale === undefined
        ? posix.dirname(from)
        : posix.join("docs", out.locale, posix.dirname(from).slice("docs/".length));
    const relative = posix.relative(dir, target);
    return relative.startsWith("../") ? relative : `./${relative}`;
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
function rewriteLine(from: string, line: string, out: Output): string {
  const definition = reRefDefinition.exec(line);
  if (definition?.[1] !== undefined && definition[2] !== undefined) {
    const dest = definition[2].replace(/^<|>$/g, "");
    const next = rewrite(from, dest, out);
    return next === undefined ? line : `${definition[1]}${next}${definition[3] ?? ""}`;
  }
  // A link that starts inside a code span (`[x](y)`) is code, not a link; a code span inside the text of a link is not.
  const code = [...line.matchAll(/(`+)[^`]*?\1/g)].map((m) => [m.index, m.index + m[0].length] as const);
  return line.replace(reInlineLink, (whole: string, open: string, dest: string, close: string, at: number) => {
    if (code.some(([start, end]) => at >= start && at < end)) {
      return whole;
    }
    const next = rewrite(from, dest.replace(/^<|>$/g, ""), out);
    return next === undefined ? whole : `${open}${next}${close}`;
  });
}

/** The lines of a document's body with the links outside fenced code rewritten, as written in the document at `from`. */
function rewriteBody(from: string, lines: string[], out: Output): string[] {
  let fence = "";
  return lines.map((line) => {
    const marker = /^\s{0,3}(`{3,}|~{3,})/.exec(line)?.[1];
    if (marker !== undefined && (fence === "" || (marker[0] === fence[0] && marker.length >= fence.length))) {
      fence = fence === "" ? marker : "";
      return line;
    }
    return fence === "" ? rewriteLine(from, line, out) : line;
  });
}

/** The title of a document from its first `# Heading`, and its lines without that heading. */
function takeTitle(path: string, lines: string[]): { title: string; rest: string[] } {
  const titleAt = lines.findIndex((line) => /^# +\S/.test(line));
  if (titleAt === -1) {
    throw new Error(`${path}: no "# Heading" to take the title from`);
  }
  const title = (lines[titleAt] ?? "")
    .replace(/^# +/, "")
    .replace(/\s*\{#[^}]*\}\s*$/, "")
    .trim();
  return { title, rest: [...lines.slice(0, titleAt), ...lines.slice(titleAt + 1)] };
}

/**
 * The page for the document at `path`: frontmatter with its title, then its body with the links rewritten; with
 * `absolute`, the plain Markdown form instead (the title as its first heading, no frontmatter, links with the origin).
 */
function transform(path: string, source: string, absolute: boolean): string {
  const { title, rest } = takeTitle(path, source.split("\n"));
  const body = rewriteBody(path, rest, { absolute });
  if (absolute) {
    return [`# ${title}`, ...body].join("\n").replace(/\n*$/, "\n");
  }
  const frontmatter = [
    "---",
    `title: ${JSON.stringify(title)}`,
    `# Generated from ${path} by app/docs/scripts/import-docs.ts.`,
    "---",
  ];
  return [...frontmatter, ...body].join("\n").replace(/\n*$/, "\n");
}

/** A section listed by the Banner of a translated page: its English heading and its place on the English page. */
interface ListedSection {
  heading: string;
  href: string;
}

/** The `translation` of a translated page's frontmatter, which the Banner reads (src/content.config.ts). */
function translationData(page: PageTranslation, english: string): object {
  const listed = (state: "stale" | "missing"): ListedSection[] =>
    page.sections.flatMap(({ section, state: s }, index) =>
      s === state
        ? [{ heading: section.heading.replaceAll("`", ""), href: index === 0 ? english : `${english}#${section.id}` }]
        : [],
    );
  return {
    source: page.source,
    english,
    legal: legalTemplates.has(page.source),
    stale: listed("stale"),
    untranslated: listed("missing"),
  };
}

/** The page of a translated document: each section of the source, translated where the translation has it. */
function translatedPage(page: PageTranslation): string {
  const lines = page.sections.flatMap(({ section, translation }) => {
    const text = (translation?.text ?? section.text).split("\n");
    // Every heading keeps the anchor of its English heading, so that the links and #anchors of the source still lead
    // to it. The first line of a section is its heading (`translations.ts check` makes sure of it for a translation).
    const heading = headingOf(text[0] ?? "");
    if (heading !== undefined) {
      text[0] = `${"#".repeat(heading.level)} ${heading.heading} {#${section.id}}`;
    }
    return [...text, ""];
  });
  const { title, rest } = takeTitle(page.file, lines);
  const body = rewriteBody(page.source, rest, { absolute: false, locale: localeSegment(page.locale) });
  const frontmatter = [
    "---",
    `title: ${JSON.stringify(title)}`,
    `translation: ${JSON.stringify(translationData(page, pageURL(page.source)))}`,
    `# Generated from ${page.file} and ${page.source} by app/docs/scripts/import-docs.ts.`,
    "---",
  ];
  return [...frontmatter, ...body].join("\n").replace(/\n*$/, "\n");
}

/**
 * A page written in the site (the landing page, the analytics notice) in a locale: its translation, with the
 * `# source:` line of its frontmatter replaced by the `translation` the Banner reads, and the paths to the site's
 * components and assets one directory deeper.
 */
function translatedHandWritten(page: PageTranslation): string {
  const source = readFileSync(join(repoRoot, page.file), "utf8");
  const lines = source.split("\n");
  const english = page.source === landingPage ? base : `${base}${posix.basename(page.source, ".mdx")}/`;
  lines[1] = `translation: ${JSON.stringify(translationData(page, english))}`;
  return lines.join("\n").replace(/(["'\s])\.\.\/\.\.\/(components|assets)\//g, "$1../../../$2/");
}

for (const name of readdirSync(contentDir)) {
  if (!handWritten.has(name)) {
    rmSync(join(contentDir, name), { recursive: true, force: true });
  }
}
for (const dir of publishedDirs) {
  rmSync(join(publicDir, dir), { recursive: true, force: true });
  rmSync(join(publicDir, `${dir}.md`), { force: true });
}
const published = publishedDirs.flatMap((dir) => filesIn(`docs/${dir}`, /\.md$/)).filter(isPublished);
for (const path of published) {
  const out = join(contentDir, `${slugOf(path)}.md`);
  mkdirSync(dirname(out), { recursive: true });
  const source = readFileSync(join(repoRoot, path), "utf8");
  writeFileSync(out, transform(path, source, false));
  const markdown = join(publicDir, markdownPath(slugOf(path)));
  mkdirSync(dirname(markdown), { recursive: true });
  writeFileSync(markdown, transform(path, source, true));
}
const images = publishedDirs.flatMap((dir) => filesIn(`docs/${dir}`, reImage));
for (const path of images) {
  const out = join(contentDir, path.slice("docs/".length));
  mkdirSync(dirname(out), { recursive: true });
  copyFileSync(join(repoRoot, path), out);
}
let translated = 0;
for (const locale of translationLocales) {
  for (const source of translatableSources()) {
    const page = translationOf(locale, source);
    if (page === undefined) {
      continue;
    }
    if (page.errors.length > 0) {
      throw new Error(page.errors.join("\n"));
    }
    const written = isHandWritten(source);
    const out = join(contentDir, localeSegment(locale), written ? posix.basename(source) : `${slugOf(source)}.md`);
    mkdirSync(dirname(out), { recursive: true });
    writeFileSync(out, written ? translatedHandWritten(page) : translatedPage(page));
    translated++;
  }
}
console.log(
  `import-docs: copied ${published.length} documents, ${images.length} images and ${translated} translations into src/content/docs/ and public/`,
);
