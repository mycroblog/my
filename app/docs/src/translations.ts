// SPDX-License-Identifier: AGPL-3.0-only
// The translations of the published documents (ADR-0150 §9): an agent translates them by hand, section by section,
// into app/docs/i18n/<locale>/, following docs/agents/translating-docs.md. A source document is split into sections at
// each heading; each translated section starts with a marker that names the section (the anchor of its English
// heading) and the hash of the English text it was translated from:
//
//   <!-- source: visibility 3f2a9c1b0d4e -->
//   ## 公開範囲
//
// A section whose English text has changed since is stale: the page keeps its old translation and lists it at the
// top. A section with no translation is shown in English. The landing page (MDX) is one section: the hash is the first
// line of its frontmatter (`# source: 3f2a9c1b0d4e`).
//
// The copy step (scripts/import-docs.ts) builds the pages of a locale from this module, and the command of
// scripts/translations.ts (`scripts/task.sh docs:site-translations`) lists what to translate and checks a translation
// with it, so both read the sections the same way.
import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join, posix } from "node:path";
import GithubSlugger from "github-slugger";
import { base, isPublished, type publishedDirs, repoRoot } from "./published";

/**
 * The locales the site is translated into, besides English at the root, as the BCP 47 tags of their catalogs in
 * locales/ and of their directories in app/docs/i18n/.
 */
export const translationLocales = ["ja", "zh-CN", "zh-TW", "es", "pt-BR", "de", "fr"] as const;
export type TranslationLocale = (typeof translationLocales)[number];

/**
 * The directory of a locale in the URL space and in src/content/docs/, which is its key in the `locales` of
 * astro.config.ts: the tag in lower case (/zh-cn/ for zh-CN), as Starlight writes them.
 */
export function localeSegment(locale: TranslationLocale): string {
  return locale.toLowerCase();
}

/** The locale of a page from its URL path (/next/ja/guide/posting/ is ja, /zh-cn/ is zh-CN), or undefined for English. */
export function localeOf(pathname: string): TranslationLocale | undefined {
  const rest = pathname.startsWith(base) ? pathname.slice(base.length) : pathname.replace(/^\//, "");
  return translationLocales.find((locale) => {
    const segment = localeSegment(locale);
    return rest === segment || rest.startsWith(`${segment}/`);
  });
}

/** The landing page, written in the site rather than copied from docs/. */
export const landingPage = "app/docs/src/content/docs/index.mdx";

/** The published documents that are templates of legal texts: their translations carry an extra notice. */
export const legalTemplates: ReadonlySet<string> = new Set([
  "docs/operations/privacy-policy-template.md",
  "docs/operations/processing-record-template.md",
]);

/** One section of a source document: its heading and the text up to the next heading. */
export interface Section {
  /** The anchor of the heading on the English page ("" for the landing page, which is one section). */
  id: string;
  /** The text of the heading, without its #s ("" for the landing page). */
  heading: string;
  /** The level of the heading, 1 to 6 (0 for the landing page). */
  level: number;
  text: string;
  hash: string;
}

/** One section of a translation: the section it translates, the hash it was translated from, and its text. */
interface TranslatedSection {
  id: string;
  hash: string;
  text: string;
  /** The line of its marker, from 1. */
  line: number;
}

type SectionState = "current" | "stale" | "missing";

/** The translation of one source document into one locale. */
export interface PageTranslation {
  locale: TranslationLocale;
  /** The repository path of the source: docs/guide/posting.md, or the landing page. */
  source: string;
  /** The repository path of the translation: app/docs/i18n/ja/guide/posting.md. */
  file: string;
  /** The sections of the source, in its order, each with its translation, if any. */
  sections: { section: Section; translation?: TranslatedSection; state: SectionState }[];
  /** The translated sections whose section the source no longer has. */
  orphans: TranslatedSection[];
  /** What is wrong with the file itself: a marker that does not parse, a section translated twice. */
  errors: string[];
}

const reFence = /^\s{0,3}(`{3,}|~{3,})/;
const reHeading = /^(#{1,6}) +(.*?)(?: +#+)? *$/;
const reMarker = /^<!-- source: (\S+) ([0-9a-f]{12}) -->$/;
const reLooseMarker = /^<!--\s*source:/;
const reLandingMarker = /^# source: ([0-9a-f]{12})$/;

/** The hash of a section's text that a marker stores: the first 12 hex digits of its SHA-256. */
function hashOf(text: string): string {
  return createHash("sha256").update(text).digest("hex").slice(0, 12);
}

/** The text a heading's anchor is made from: the heading without the Markdown of its links and code spans. */
function plainHeading(heading: string): string {
  return heading.replace(/!?\[([^\]]*)\]\([^)]*\)/g, "$1").replaceAll("`", "");
}

/** Calls `visit` with each line of `lines` and whether it is outside a fenced code block (a fence line is inside). */
function eachLine(lines: string[], visit: (line: string, outside: boolean, index: number) => void): void {
  let fence = "";
  lines.forEach((line, index) => {
    const marker = reFence.exec(line)?.[1];
    if (marker !== undefined && (fence === "" || (marker[0] === fence[0] && marker.length >= fence.length))) {
      fence = fence === "" ? marker : "";
      visit(line, false, index);
      return;
    }
    visit(line, fence === "", index);
  });
}

/** The text of a run of lines, without the blank lines at its end. */
function joinLines(lines: string[]): string {
  return lines.join("\n").replace(/\s+$/, "");
}

/**
 * The sections of a source document, split at each heading outside fenced code: each section is its heading and the
 * lines up to the next heading. Lines before the first heading belong to the first section. The landing page is one
 * section.
 */
function sectionsOf(path: string, source: string): Section[] {
  if (path.endsWith(".mdx")) {
    const text = joinLines(source.split("\n"));
    return [{ id: "", heading: "", level: 0, text, hash: hashOf(text) }];
  }
  const slugger = new GithubSlugger();
  const runs: { heading: string; level: number; lines: string[] }[] = [];
  const before: string[] = [];
  eachLine(source.split("\n"), (line, outside) => {
    const heading = outside ? reHeading.exec(line) : null;
    if (heading?.[1] !== undefined && heading[2] !== undefined) {
      runs.push({ heading: heading[2], level: heading[1].length, lines: [line] });
      return;
    }
    (runs.at(-1)?.lines ?? before).push(line);
  });
  const first = runs[0];
  if (first !== undefined && before.some((line) => line.trim() !== "")) {
    first.lines.unshift(...before);
  }
  return runs.map(({ heading, level, lines }) => {
    const text = joinLines(lines);
    return { id: slugger.slug(plainHeading(heading)), heading, level, text, hash: hashOf(text) };
  });
}

/** The sections of the source document at the repository path `source`. */
export function sourceSections(source: string): Section[] {
  return sectionsOf(source, readFileSync(join(repoRoot, source), "utf8"));
}

/** The repository path of the translation of `source` into `locale`. */
export function translationPath(locale: TranslationLocale, source: string): string {
  if (source === landingPage) {
    return `app/docs/i18n/${locale}/index.mdx`;
  }
  return `app/docs/i18n/${locale}/${source.slice("docs/".length)}`;
}

/** The sections of a translation file, and what does not parse. */
function parseTranslation(file: string, source: string): { sections: TranslatedSection[]; errors: string[] } {
  const lines = source.split("\n");
  if (file.endsWith(".mdx")) {
    const hash = lines[0] === "---" ? reLandingMarker.exec(lines[1] ?? "")?.[1] : undefined;
    if (hash === undefined) {
      return { sections: [], errors: [`${file}:2: the frontmatter does not start with "# source: <hash>"`] };
    }
    return { sections: [{ id: "", hash, text: joinLines(lines), line: 2 }], errors: [] };
  }
  const sections: TranslatedSection[] = [];
  const errors: string[] = [];
  let open: { id: string; hash: string; line: number; lines: string[] } | undefined;
  const close = (): void => {
    if (open !== undefined) {
      sections.push({ id: open.id, hash: open.hash, line: open.line, text: joinLines(open.lines) });
    }
  };
  eachLine(lines, (line, outside, index) => {
    const marker = outside ? reMarker.exec(line) : null;
    if (marker?.[1] !== undefined && marker[2] !== undefined) {
      close();
      open = { id: marker[1], hash: marker[2], line: index + 1, lines: [] };
      return;
    }
    if (outside && reLooseMarker.test(line)) {
      errors.push(`${file}:${index + 1}: a marker must read "<!-- source: <section> <12 hex digits> -->"`);
    }
    if (open === undefined) {
      if (line.trim() !== "") {
        errors.push(`${file}:${index + 1}: text before the first "<!-- source: ... -->" marker`);
      }
      return;
    }
    open.lines.push(line);
  });
  close();
  const seen = new Set<string>();
  for (const section of sections) {
    if (seen.has(section.id)) {
      errors.push(`${file}:${section.line}: the section ${section.id} is translated twice`);
    }
    seen.add(section.id);
  }
  return { sections, errors };
}

/** The translation of `source` into `locale`, or undefined when the locale has no file for it. */
export function translationOf(locale: TranslationLocale, source: string): PageTranslation | undefined {
  const file = translationPath(locale, source);
  const absolute = join(repoRoot, file);
  if (!existsSync(absolute)) {
    return undefined;
  }
  const parsed = parseTranslation(file, readFileSync(absolute, "utf8"));
  const byId = new Map(parsed.sections.map((section) => [section.id, section]));
  const sections = sourceSections(source).map((section) => {
    const translation = byId.get(section.id);
    byId.delete(section.id);
    if (translation === undefined) {
      return { section, state: "missing" as const };
    }
    return {
      section,
      translation,
      state: translation.hash === section.hash ? ("current" as const) : ("stale" as const),
    };
  });
  return { locale, source, file, sections, orphans: [...byId.values()], errors: parsed.errors };
}

/** The repository paths of the files under `dir` (relative to the repository) whose name matches `suffix`, sorted. */
export function filesIn(dir: string, suffix: RegExp): string[] {
  const absolute = join(repoRoot, dir);
  if (!existsSync(absolute)) {
    return [];
  }
  return readdirSync(absolute, { recursive: true, encoding: "utf8" })
    .map((name) => posix.join(dir, name.split("\\").join("/")))
    .filter((path) => suffix.test(path) && statSync(join(repoRoot, path)).isFile())
    .sort();
}

/**
 * The published directories whose documents are translated. docs/reference (mostly names of settings, commands and
 * metrics, partly generated), docs/developers and docs/plugins (read by developers of clients and plugins, in English)
 * are not: their pages in a locale are the English pages with Starlight's notice.
 */
const translatedDirs: readonly (typeof publishedDirs)[number][] = ["guide", "operations"];

/** Every source a locale translates: the landing page and the published documents of translatedDirs. */
export function translatableSources(): string[] {
  return [landingPage, ...translatedDirs.flatMap((dir) => filesIn(`docs/${dir}`, /\.md$/)).filter(isPublished)];
}

/** The files under app/docs/i18n/<locale>/ that translate no source: a source that was moved or removed. */
export function strayTranslations(locale: TranslationLocale): string[] {
  const expected = new Set(translatableSources().map((source) => translationPath(locale, source)));
  return filesIn(`app/docs/i18n/${locale}`, /\.mdx?$/).filter((file) => !expected.has(file));
}

/** The level of the first line of a section's text if it is a heading, and the heading itself. */
export function headingOf(text: string): { level: number; heading: string } | undefined {
  const match = reHeading.exec(text.split("\n", 1)[0] ?? "");
  return match?.[1] === undefined || match[2] === undefined ? undefined : { level: match[1].length, heading: match[2] };
}

/**
 * The parts of a Markdown text a translation must keep as they are, outside code blocks and in them. Code spans and
 * links are read per paragraph, since a code span may continue on the next line (CommonMark reads the line break as a
 * space): the English source wraps its lines, and a translation need not wrap them at the same place.
 */
export function protectedParts(text: string): {
  headings: number[];
  fences: string[];
  codeSpans: string[];
  links: string[];
} {
  const headings: number[] = [];
  const fences: string[] = [];
  const codeSpans: string[] = [];
  const links: string[] = [];
  let block: string[] | undefined;
  let paragraph: string[] = [];
  const flush = (): void => {
    if (paragraph.length === 0) {
      return;
    }
    const joined = paragraph.join(" ");
    paragraph = [];
    for (const match of joined.matchAll(/(`+)(.+?)\1/g)) {
      codeSpans.push(match[2] ?? "");
    }
    const withoutCode = joined.replace(/(`+).+?\1/g, "");
    for (const match of withoutCode.matchAll(/\]\((<[^>]*>|[^)\s]+)/g)) {
      links.push((match[1] ?? "").replace(/^<|>$/g, ""));
    }
  };
  eachLine(text.split("\n"), (line, outside) => {
    if (!outside) {
      flush();
      if (reFence.test(line) && block !== undefined) {
        block.push(line);
        fences.push(block.join("\n"));
        block = undefined;
      } else if (block === undefined) {
        block = [line];
      } else {
        block.push(line);
      }
      return;
    }
    const heading = reHeading.exec(line);
    const definition = /^\s{0,3}\[[^\]]+\]:\s*(<[^>]*>|\S+)/.exec(line);
    if (heading?.[1] !== undefined || definition !== null || line.trim() === "") {
      flush();
    }
    if (heading?.[1] !== undefined) {
      headings.push(heading[1].length);
    }
    if (definition?.[1] !== undefined) {
      links.push(definition[1].replace(/^<|>$/g, ""));
      return;
    }
    if (line.trim() !== "") {
      paragraph.push(line.trim());
    }
    if (heading?.[1] !== undefined) {
      flush();
    }
  });
  flush();
  return { headings, fences, codeSpans: codeSpans.sort(), links: links.sort() };
}

/** The title of the translation of `source` into `locale`: the heading of its translated first section, if any. */
export function translatedTitle(locale: TranslationLocale, source: string): string | undefined {
  const first = translationOf(locale, source)?.sections[0]?.translation;
  return first === undefined ? undefined : headingOf(first.text)?.heading;
}

/**
 * The labels of the sidebar's groups in `locale`, by the English `## Heading` or `### Heading` of docs/README.md they
 * translate (app/docs/i18n/<locale>/sidebar.json). A heading missing there keeps its English label.
 */
export function sidebarLabels(locale: TranslationLocale): Record<string, string> {
  const file = join(repoRoot, `app/docs/i18n/${locale}/sidebar.json`);
  return existsSync(file) ? (JSON.parse(readFileSync(file, "utf8")) as Record<string, string>) : {};
}

/** The messages of a catalog of locales/<locale>/, by key: every namespace, client and server. */
function messagesOf(locale: string): Map<string, string> {
  const messages = new Map<string, string>();
  for (const file of filesIn(`locales/${locale}`, /\.json$/)) {
    const catalog = JSON.parse(readFileSync(join(repoRoot, file), "utf8")) as Record<
      string,
      { defaultMessage?: unknown }
    >;
    for (const [key, entry] of Object.entries(catalog)) {
      if (typeof entry.defaultMessage === "string") {
        messages.set(key, entry.defaultMessage);
      }
    }
  }
  return messages;
}

/**
 * The UI terms of `locale` by their English text: for each English message of locales/en, the messages of the same
 * keys in locales/<locale>/ (more than one when the same English text is translated differently in places), and
 * each message with simple arguments also by its text with "..." in place of each argument. The
 * documents name the UI by its English text in quotes ("Write a post"), and a translation names it by these.
 */
export function uiTerms(locale: TranslationLocale): Map<string, string[]> {
  const english = messagesOf("en");
  const translated = messagesOf(locale);
  const terms = new Map<string, string[]>();
  const add = (text: string, term: string): void => {
    const known = terms.get(text) ?? [];
    if (!known.includes(term)) {
      known.push(term);
    }
    terms.set(text, known);
  };
  for (const [key, text] of english) {
    const term = translated.get(key);
    if (term === undefined) {
      continue;
    }
    add(text, term);
    // The documents write a simple argument of a message ("Unfollow {name}?") as "...": "Unfollow ...?".
    const placeholder = /\{\w+\}/g;
    if (placeholder.test(text) && !/\{\w+,/.test(text)) {
      add(text.replace(placeholder, "..."), term.replace(placeholder, "…"));
    }
  }
  return terms;
}

/** The quoted phrases ("Write a post") of a Markdown text outside code, in order, each once; a phrase may wrap a line. */
export function quotedPhrases(text: string): string[] {
  // Paragraphs, list items and table rows, each one string: a blank line, code, or a new item starts the next.
  const paragraphs: string[][] = [[]];
  eachLine(text.split("\n"), (line, outside) => {
    if (!outside || line.trim() === "" || /^\s*([-*|]|\d+\.)\s/.test(line)) {
      paragraphs.push([]);
    }
    if (outside) {
      paragraphs.at(-1)?.push(line.replace(/(`+).+?\1/g, ""));
    }
  });
  const phrases: string[] = [];
  for (const paragraph of paragraphs.map((lines) => lines.join(" "))) {
    for (const match of paragraph.matchAll(/"([^"]{1,80})"/g)) {
      const phrase = (match[1] ?? "").replace(/\s+/g, " ");
      if (!phrases.includes(phrase)) {
        phrases.push(phrase);
      }
    }
  }
  return phrases;
}
