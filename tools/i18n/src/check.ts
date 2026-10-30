// SPDX-License-Identifier: AGPL-3.0-only
// Catalog checks run by `scripts/task.sh i18n:check` (ADR-0009 §1).
//   error: ICU syntax error, including a plural / select without `other`
//   error: translation argument unknown to the source, or of a different type
//   error: plural / selectordinal category the locale's CLDR rules do not have
//   error: translation key absent from the source (obsolete)
//   error: rich-text tag, number skeleton / style, or date / time argument in a namespace
//          shared with Go (anything but client*)
//   error: malformed locale directory, namespace, or key; two keys mapping to one accessor
//   error: catalog file outside locales/<locale>/<namespace>/<area>.json, or a key outside its area's file (ADR-0082)
//   error: generated files Go reads (server pseudo-locales, Go accessors, golden) differ from `scripts/task.sh i18n:gen`
// The generated files only the TS clients read (client pseudo-locales, TS accessors) are not committed and not
// checked here (ADR-0081): i18n:gen-ts writes them before the tasks that read them, maybe while this runs.
//   error: copy outside the tone and punctuation rules (web-client/16-look-states-and-wording.md G19, G20): an exclamation mark
//          in any non-pseudo locale; a full-width colon, semicolon or parenthesis, or an ASCII comma, in Japanese text
//          (use ":", ";", "()" and "、")
//   error: a variant the glossary (tools/i18n/glossary.json, G21) bans in the locale's text, in any namespace
//   warn:  untranslated key (falls back to en at runtime)
//   warn:  translation drops a source argument (sometimes legitimate)
//   warn:  key not referenced from code
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import {
  areaOf,
  type Catalog,
  type CatalogFile,
  catalogFile,
  formattingLocale,
  isClientNamespace,
  isPseudo,
  loadCatalog,
  loadCatalogFiles,
  rel,
  repoRoot,
  SOURCE,
} from "./catalog";
import { generate, goName, pseudoPart } from "./generate";
import { glossaryFindings, loadGlossary } from "./glossary";
import { type Args, extractArgs, literalText, outsideSharedSubset, parseMessage, pluralCategories } from "./icu";

let errors = 0;
let warnings = 0;
const error = (m: string) => (errors++, console.error(`error: ${m}`));
const warn = (m: string) => (warnings++, console.warn(`warn:  ${m}`));

const NAMESPACE = /^[a-z][a-z0-9]*$/;
/** "locales/<locale>/<namespace>/<area>.json <key>", where a finding about a key points. */
const at = (locale: string, ns: string, key: string): string => `${rel(catalogFile(locale, ns, key))} ${key}`;
const KEY = /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$/;
const glossary = loadGlossary();

function analyze(locale: string, ns: string, key: string, message: unknown): Args | undefined {
  const where = at(locale, ns, key);
  if (typeof message !== "string") return void error(`${where}: defaultMessage must be a string`);
  try {
    const ast = parseMessage(message);
    if (!isClientNamespace(ns)) {
      for (const f of outsideSharedSubset(ast)) {
        error(`${where}: ${f} is outside the Go/TS shared subset (allowed only in client* namespaces)`);
      }
    }
    const rules = {
      cardinal: new Set<string>(new Intl.PluralRules(formattingLocale(locale)).resolvedOptions().pluralCategories),
      ordinal: new Set<string>(
        new Intl.PluralRules(formattingLocale(locale), { type: "ordinal" }).resolvedOptions().pluralCategories,
      ),
    };
    for (const f of copyStyleFindings(locale, literalText(ast))) error(`${where}: ${f}`);
    for (const c of pluralCategories(ast)) {
      if (!rules[c.type].has(c.key)) error(`${where}: ${c.type} category "${c.key}" does not exist in ${locale}`);
    }
    return extractArgs(ast);
  } catch (e) {
    error(`${where}: ${(e as Error).message}`);
  }
}

/** The glossary variants (G21) and the punctuation that web-client/16-look-states-and-wording.md G19 / G20 rule out, found in the message's own text (not its ICU syntax). */
function copyStyleFindings(locale: string, text: string): string[] {
  if (isPseudo(locale)) return [];
  const out: string[] = glossaryFindings(glossary, locale, text);
  if (/[!！]/.test(text)) out.push("no exclamation marks in UI copy (web-client/16-look-states-and-wording.md G19)");
  if (new Intl.Locale(locale).language === "ja") {
    if (/[：；（）]/u.test(text))
      out.push(
        'use the ASCII colon, semicolon and parentheses (": ", "; ", " (…)") in Japanese, not "：", "；", "（）" (web-client/16-look-states-and-wording.md G20)',
      );
    if (text.includes(","))
      out.push('use "、" instead of an ASCII comma in Japanese (web-client/16-look-states-and-wording.md G20)');
  }
  return out;
}

function checkCatalog(catalog: Catalog) {
  if (!catalog[SOURCE]) return void error(`locales/${SOURCE}: the source locale is missing`);
  for (const locale of Object.keys(catalog)) {
    let canonical: string | undefined;
    try {
      canonical = Intl.getCanonicalLocales(locale)[0];
    } catch {}
    if (canonical !== locale)
      error(`locales/${locale}: not a canonical BCP 47 tag${canonical ? ` (use ${canonical})` : ""}`);
  }

  const sourceArgs = new Map<string, Args | undefined>();
  const accessors = new Map<string, string>();
  for (const [ns, entries] of Object.entries(catalog[SOURCE])) {
    if (!NAMESPACE.test(ns)) error(`locales/${SOURCE}/${ns}/: namespace must match ${NAMESPACE}`);
    for (const [key, e] of Object.entries(entries)) {
      const id = `${ns}:${key}`;
      if (!KEY.test(key)) error(`${at(SOURCE, ns, key)}: key must match ${KEY}`);
      if (!e.description) warn(`${at(SOURCE, ns, key)}: no description for translators`);
      const name = goName(id);
      if (accessors.has(name)) error(`${id} and ${accessors.get(name)} both map to the accessor ${name}`);
      accessors.set(name, id);
      sourceArgs.set(id, analyze(SOURCE, ns, key, e.defaultMessage));
    }
  }

  for (const [locale, namespaces] of Object.entries(catalog)) {
    if (locale === SOURCE) continue;
    for (const id of sourceArgs.keys()) {
      const [ns, key] = id.split(/:(.*)/s);
      if (isPseudo(locale) && pseudoPart(ns) !== "go") continue;
      if (!namespaces[ns]?.[key]) warn(`${locale}: untranslated ${id}`);
    }
    for (const [ns, entries] of Object.entries(namespaces)) {
      for (const [key, e] of Object.entries(entries)) {
        const id = `${ns}:${key}`;
        if (!sourceArgs.has(id)) {
          error(`${at(locale, ns, key)}: not in the source locale (obsolete)`);
          continue;
        }
        const want = sourceArgs.get(id);
        const got = analyze(locale, ns, key, e.defaultMessage);
        if (!want || !got) continue;
        for (const [name, t] of got) {
          const w = want.get(name);
          if (!w) error(`${at(locale, ns, key)}: unknown argument {${name}}`);
          else if (w.kind !== t.kind) error(`${at(locale, ns, key)}: {${name}} is ${t.kind}, the source has ${w.kind}`);
        }
        for (const name of want.keys()) {
          if (!got.has(name) && !isPseudo(locale)) warn(`${at(locale, ns, key)}: drops argument {${name}}`);
        }
      }
    }
  }
  return sourceArgs;
}

/**
 * Warns about keys referenced neither by a Go accessor (a call, a method expression such as
 * `(*i18n.Localizer).ServerX`, or a field chain such as `.L.ServerX` in an html/template file)
 * nor by a TS key literal.
 */
function checkUnused(ids: Iterable<string>) {
  const read = (pattern: string, skip: (p: string) => boolean) =>
    [...new Bun.Glob(pattern).scanSync({ cwd: repoRoot })]
      .filter((p) => !p.includes("node_modules/") && !p.startsWith("internal/i18n/") && !skip(p))
      .map((p) => readFileSync(join(repoRoot, p), "utf8"))
      .join("\n");
  const go = read("{cmd,internal,pkg}/**/*.go", (p) => p.endsWith("_gen.go") || p.endsWith("_test.go"));
  const templates = read("internal/**/*.html", () => false);
  const ts = existsSync(join(repoRoot, "app")) ? read("app/**/*.{ts,tsx}", (p) => p.endsWith(".gen.ts")) : "";
  for (const id of ids) {
    const name = goName(id);
    const usedByGo =
      go.includes(`.${name}(`) || go.includes(`).${name}`) || new RegExp(`\\.${name}(?![A-Za-z0-9_])`).test(templates);
    const used = (!isClientNamespace(id) && usedByGo) || ts.includes(JSON.stringify(id));
    if (!used) warn(`unused ${id}`);
  }
}

/** Every catalog file is locales/<locale>/<namespace>/<area>.json and holds only the keys of its area (ADR-0082). */
function checkLayout(files: CatalogFile[]) {
  for (const { path, locale, ns, area, entries } of files) {
    if (!ns) {
      error(`${rel(path)}: catalogs are locales/<locale>/<namespace>/<area>.json; split it by area`);
      continue;
    }
    for (const key of Object.keys(entries)) {
      if (areaOf(key) !== area) error(`${rel(path)} ${key}: belongs in ${rel(catalogFile(locale, ns, key))}`);
    }
  }
}

async function checkGenerated(catalog: Catalog, files: CatalogFile[]) {
  const generated = await generate(catalog, ["go"]);
  for (const [path, want] of generated) {
    const got = existsSync(path) ? readFileSync(path, "utf8") : undefined;
    if (got !== want)
      error(
        `${rel(path)} is ${got === undefined ? "missing" : "stale"}; run scripts/task.sh i18n:gen and commit the result`,
      );
  }
  for (const { path } of files.filter((f) => isPseudo(f.locale))) {
    if (!generated.has(path)) error(`${rel(path)} has no source; run scripts/task.sh i18n:gen`);
  }
}

// The client pseudo-locales are generated, uncommitted files that may be missing or stale; leave them out.
const files = loadCatalogFiles().filter((f) => !isPseudo(f.locale) || (f.ns !== "" && pseudoPart(f.ns) === "go"));
const catalog = loadCatalog(files);
checkLayout(files);
const sourceArgs = checkCatalog(catalog);
if (errors === 0) {
  checkUnused(sourceArgs?.keys() ?? []);
  await checkGenerated(catalog, files);
}
console.log(`i18n: ${errors} error(s), ${warnings} warning(s)`);
if (errors > 0) process.exit(1);
