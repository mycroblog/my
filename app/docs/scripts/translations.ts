// SPDX-License-Identifier: AGPL-3.0-only
// The command an agent translates the documents with (ADR-0150 §9, docs/agents/translating-docs.md), run as
// `scripts/task.sh docs:site-translations -- <command>`:
//
//   status [--locale ja] [source...]   what each source still needs in a locale: no translation file, or the sections
//                                      that are missing or stale, and the translated sections the source no longer has
//   todo [--locale ja] <source>        the sections of one source to translate, each with the marker to write above it
//                                      and its English text
//   glossary [--locale ja] <source>    the UI terms the source quotes ("Write a post") and their text in the locale's
//                                      catalog (locales/<locale>/), which the translation uses for them
//   check [--locale ja]                the translation files of the locale, or of every locale without --locale, are
//                                      sound: their markers parse, each section starts with a heading of its English
//                                      level, and a current section keeps the code blocks, code spans and link targets
//                                      of its English text; files that translate nothing
//
// A source is a repository path: docs/guide/posting.md, or a page written in the site: app/docs/src/content/docs/index.mdx
// (the landing page) or app/docs/src/content/docs/analytics.mdx (the analytics notice).
// status and todo always succeed; check fails on what it finds. A stale or missing section is never a failure: the
// page keeps the old translation, or shows the English text, and says so.
import {
  headingOf,
  isHandWritten,
  protectedParts,
  quotedPhrases,
  sourceSections,
  strayTranslations,
  translatableSources,
  type TranslationLocale,
  translationLocales,
  translationOf,
  translationPath,
  uiTerms,
} from "../src/translations";

function usage(): never {
  console.error("usage: translations.ts status|todo|glossary|check [--locale ja] [source...]");
  process.exit(2);
}

const args = process.argv.slice(2);
const command = args.shift() ?? usage();
let locale: TranslationLocale = "ja";
let localeGiven = false;
const sources: string[] = [];
while (args.length > 0) {
  const arg = args.shift() ?? "";
  if (arg === "--locale") {
    const value = args.shift() ?? usage();
    if (!(translationLocales as readonly string[]).includes(value)) {
      console.error(`translations.ts: the site has no locale ${value} (${translationLocales.join(", ")})`);
      process.exit(2);
    }
    locale = value as TranslationLocale;
    localeGiven = true;
  } else {
    sources.push(arg.replace(/^\.\//, ""));
  }
}

const known = new Set(translatableSources());
for (const source of sources) {
  if (!known.has(source)) {
    console.error(
      `translations.ts: ${source} is not a document the site translates (docs/guide, docs/operations, the pages written in the site)`,
    );
    process.exit(2);
  }
}

function status(): void {
  let complete = 0;
  const lines: string[] = [];
  for (const source of sources.length > 0 ? sources : translatableSources()) {
    const page = translationOf(locale, source);
    if (page === undefined) {
      lines.push(`${source}: not translated (${translationPath(locale, source)})`);
      continue;
    }
    const missing = page.sections.filter((s) => s.state === "missing").map((s) => s.section.id || "(page)");
    const stale = page.sections.filter((s) => s.state === "stale").map((s) => s.section.id || "(page)");
    const orphans = page.orphans.map((s) => s.id);
    if (missing.length + stale.length + orphans.length === 0) {
      complete++;
      if (sources.length > 0) {
        lines.push(`${source}: up to date`);
      }
      continue;
    }
    const parts = [
      ...(stale.length > 0 ? [`stale: ${stale.join(", ")}`] : []),
      ...(missing.length > 0 ? [`missing: ${missing.join(", ")}`] : []),
      ...(orphans.length > 0 ? [`no longer in the source: ${orphans.join(", ")}`] : []),
    ];
    lines.push(`${source}: ${parts.join("; ")}`);
  }
  for (const line of lines) {
    console.log(line);
  }
  if (sources.length === 0) {
    console.log(`${locale}: ${complete} of ${translatableSources().length} sources up to date`);
  }
}

function todo(): void {
  const source = sources.length === 1 ? sources[0] : undefined;
  if (source === undefined) {
    usage();
  }
  const page = translationOf(locale, source);
  const landing = isHandWritten(source);
  const sections = page?.sections ?? sourceSections(source).map((section) => ({ section, state: "missing" as const }));
  console.log(`# Translate into ${translationPath(locale, source)}, keeping the sections in the order below.`);
  if (page === undefined) {
    console.log("# The file does not exist yet: every section is to be translated.");
  }
  if (page !== undefined && page.orphans.length > 0) {
    console.log(`# Remove the sections the source no longer has: ${page.orphans.map((s) => s.id).join(", ")}.`);
  }
  for (const { section, state } of sections) {
    if (state === "current") {
      continue;
    }
    const what = state === "stale" ? "Stale, translate again" : "Missing, translate";
    console.log("");
    if (landing) {
      console.log(`# ${what} the whole page. Its frontmatter starts with the line:`);
      console.log(`# source: ${section.hash}`);
    } else {
      console.log(`# ${what}: the section ${section.id}, under the marker`);
      console.log(`<!-- source: ${section.id} ${section.hash} -->`);
    }
    console.log(section.text);
  }
}

function glossary(): void {
  const terms = uiTerms(locale);
  const text = sources
    .map((source) =>
      sourceSections(source)
        .map((section) => section.text)
        .join("\n"),
    )
    .join("\n");
  if (sources.length === 0) {
    usage();
  }
  for (const phrase of quotedPhrases(text)) {
    const bare = phrase.replace(/(\.\.\.|[.…])$/, "");
    const found = terms.get(phrase) ?? terms.get(bare) ?? terms.get(`${bare}.`) ?? terms.get(`${bare}…`);
    if (found !== undefined) {
      console.log(`"${phrase}" → ${found.map((t) => `「${t}」`).join(" or ")}`);
      continue;
    }
    // Messages that contain the phrase: a plural count or a longer sentence around it ("Speakers · {count, number}").
    const near = [...terms].filter(([english]) => english.includes(bare)).slice(0, 3);
    console.log(
      near.length === 0
        ? `"${phrase}" → (no UI string: translate it)`
        : `"${phrase}" → no exact UI string; near: ${near.map(([english, ts]) => `"${english}" 「${ts.join("」 or 「")}」`).join("; ")}`,
    );
  }
}

function check(locale: TranslationLocale): number {
  const errors: string[] = [];
  for (const file of strayTranslations(locale)) {
    errors.push(`${file}: translates no published document (moved or removed?)`);
  }
  for (const source of translatableSources()) {
    const page = translationOf(locale, source);
    if (page === undefined) {
      continue;
    }
    errors.push(...page.errors);
    for (const { section, translation, state } of page.sections) {
      if (translation === undefined || isHandWritten(source)) {
        continue;
      }
      const where = `${page.file}:${translation.line}`;
      const heading = headingOf(translation.text);
      if (heading === undefined) {
        errors.push(`${where}: the section ${section.id} does not start with its heading`);
        continue;
      }
      // A stale section is checked again once it is translated again: a change to the English text alone never fails.
      if (state !== "current") {
        continue;
      }
      if (heading.level !== section.level) {
        errors.push(
          `${where}: the heading of ${section.id} is level ${heading.level}, the English one ${section.level}`,
        );
      }
      const english = protectedParts(section.text);
      const translated = protectedParts(translation.text);
      if (translated.headings.length !== 1) {
        errors.push(
          `${where}: the section ${section.id} has ${translated.headings.length} headings; split it at each English heading`,
        );
      }
      const compare = (what: string, a: string[], b: string[]): void => {
        if (JSON.stringify(a) !== JSON.stringify(b)) {
          const lost = a.filter((x) => !b.includes(x));
          const added = b.filter((x) => !a.includes(x));
          const detail = [
            ...(lost.length > 0 ? [`missing ${JSON.stringify(lost)}`] : []),
            ...(added.length > 0 ? [`not in English ${JSON.stringify(added)}`] : []),
          ].join(", ");
          errors.push(
            `${where}: the ${what} of ${section.id} differ from the English text${detail === "" ? " (in number or order)" : `: ${detail}`}`,
          );
        }
      };
      compare("code blocks", english.fences, translated.fences);
      compare("code spans", english.codeSpans, translated.codeSpans);
      compare("link targets", english.links, translated.links);
    }
  }
  for (const error of errors) {
    console.error(error);
  }
  if (errors.length === 0) {
    console.log(`translations.ts check: the ${locale} translations are sound`);
  }
  return errors.length === 0 ? 0 : 1;
}

switch (command) {
  case "status":
    status();
    break;
  case "todo":
    todo();
    break;
  case "glossary":
    glossary();
    break;
  case "check":
    process.exit(
      (localeGiven ? [locale] : translationLocales).map((each) => check(each)).reduce((a, b) => Math.max(a, b), 0),
    );
    break;
  default:
    usage();
}
