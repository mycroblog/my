// SPDX-License-Identifier: AGPL-3.0-only
// Catalog checks run by `scripts/task.sh i18n:check` (ADR-0009 §1), and, for the keys of a change, by
// `scripts/i18n.sh check --changed` and by the commands of cli.ts that write a message.
//   error: ICU syntax error, including a plural / select without `other`
//   error: translation argument unknown to the source, or of a different type
//   error: plural / selectordinal category the locale's CLDR rules do not have
//   error: translation key absent from the source (obsolete)
//   error: rich-text tag, number skeleton / style, or date / time argument in a namespace
//          shared with Go (anything but client*)
//   error: malformed locale directory, namespace, or key; two keys mapping to one accessor
//   error: catalog file outside locales/<locale>/<namespace>/<area>.json, or a key outside its area's file (ADR-0081)
//   error: generated files Go reads (server pseudo-locales, Go accessors, golden) differ from `scripts/task.sh i18n:gen`
// The generated files only the TS clients read (client pseudo-locales, TS accessors) are not committed and not
// checked here (ADR-0081): i18n:gen-ts writes them before the tasks that read them, maybe while this runs.
//   error: copy outside the tone and punctuation rules (web-client/16-look-states-and-wording.md G19, G20): an exclamation mark
//          in any non-pseudo locale; a full-width colon, semicolon or parenthesis, or an ASCII comma, in Japanese text
//          (use ":", ";", "()" and "、"); in Simplified Chinese, an ASCII comma, semicolon, colon, question mark or
//          parenthesis next to a Han character, corner brackets 「」『』, or kana; in Traditional Chinese, the same ASCII
//          punctuation, curly quotation marks “”‘’ (use 「」『』), kana, or a character only Simplified Chinese writes;
//          in Spanish, a question mark without its opening "¿", or ASCII double quotation marks (use «…»); in
//          Portuguese, ASCII double quotation marks (use “…”); in German, ASCII double quotation marks (use „…“); in
//          French, ASCII double quotation marks (use « … »), ":", ";" or "?" without the narrow no-break space before
//          it, or « » without the narrow no-break space inside them
//   error: a variant the glossary (tools/i18n/glossary.json, G21) bans in the locale's text, in any namespace
//   error: key of the source missing in ja, which is written in the same change as en (`scripts/i18n.sh set`)
//   error: helper text of a client (a key ending in description / hint / help / note) longer than its budget, and
//          an entry of tools/i18n/helper-text-baseline.json that no longer needs to be there (budget.ts)
//   warn:  untranslated key in a machine-translated locale (falls back to en at runtime)
//   warn:  translation drops a source argument (sometimes legitimate)
//   warn:  key not referenced from code
import { existsSync, readFileSync } from "node:fs";
import { budgetFindings, loadBaseline } from "./budget";
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
  REQUIRED_LOCALES,
  SOURCE,
} from "./catalog";
import { generate, goName, pseudoPart } from "./generate";
import { glossaryFindings, loadGlossary } from "./glossary";
import { type Args, extractArgs, literalText, outsideSharedSubset, parseMessage, pluralCategories } from "./icu";
import { References } from "./refs";

export type Finding = { level: "error" | "warn"; message: string };
let findings: Finding[] = [];
const error = (m: string) => void findings.push({ level: "error", message: m });
const warn = (m: string) => void findings.push({ level: "warn", message: m });

export const NAMESPACE = /^[a-z][a-z0-9]*$/;
/** "locales/<locale>/<namespace>/<area>.json <key>", where a finding about a key points. */
const at = (locale: string, ns: string, key: string): string => `${rel(catalogFile(locale, ns, key))} ${key}`;
export const KEY = /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$/;
const glossary = loadGlossary();

/**
 * The errors of one message on its own (ICU syntax, the shared subset, the copy rules, the plural categories of its
 * locale) and of its arguments against the source message `source`, without the path they are reported under.
 * `dropped` lists the source arguments it leaves out, which the catalog check only warns about.
 */
export function messageFindings(
  locale: string,
  ns: string,
  message: string,
  source?: string,
): { errors: string[]; dropped: string[] } {
  const collected = findings;
  findings = [];
  try {
    const got = analyze(locale, ns, "", message, "");
    const dropped: string[] = [];
    if (got && source !== undefined && locale !== SOURCE) {
      let want: Args | undefined;
      try {
        want = extractArgs(parseMessage(source));
      } catch {}
      if (want) dropped.push(...compareArgs("", want, got));
    }
    return { errors: findings.map((f) => f.message), dropped };
  } finally {
    findings = collected;
  }
}

/** Reports the arguments of a translation the source does not have or types differently; returns the ones it drops. */
function compareArgs(where: string, want: Args, got: Args): string[] {
  for (const [name, t] of got) {
    const w = want.get(name);
    if (!w)
      error(
        `${where}unknown argument {${name}}; the source has ${[...want.keys()].map((n) => `{${n}}`).join(", ") || "no arguments"}`,
      );
    else if (w.kind !== t.kind) error(`${where}{${name}} is ${t.kind}, the source has ${w.kind}`);
  }
  return [...want.keys()].filter((name) => !got.has(name));
}

function analyze(
  locale: string,
  ns: string,
  key: string,
  message: unknown,
  where = `${at(locale, ns, key)}: `,
): Args | undefined {
  if (typeof message !== "string") return void error(`${where}defaultMessage must be a string`);
  try {
    const ast = parseMessage(message);
    if (!isClientNamespace(ns)) {
      for (const f of outsideSharedSubset(ast)) {
        error(`${where}${f} is outside the Go/TS shared subset (allowed only in client* namespaces)`);
      }
    }
    const rules = {
      cardinal: new Set<string>(new Intl.PluralRules(formattingLocale(locale)).resolvedOptions().pluralCategories),
      ordinal: new Set<string>(
        new Intl.PluralRules(formattingLocale(locale), { type: "ordinal" }).resolvedOptions().pluralCategories,
      ),
    };
    for (const f of copyStyleFindings(locale, literalText(ast))) error(`${where}${f}`);
    for (const c of pluralCategories(ast)) {
      if (!rules[c.type].has(c.key))
        error(
          `${where}${c.type} category "${c.key}" does not exist in ${locale}; it has ${[...rules[c.type]].join(", ")} (and exact selectors such as =0)`,
        );
    }
    return extractArgs(ast);
  } catch (e) {
    error(`${where}ICU syntax: ${(e as Error).message}`);
  }
}

/** The glossary variants (G21) and the punctuation that web-client/16-look-states-and-wording.md G19 / G20 rule out, found in the message's own text (not its ICU syntax). */
function copyStyleFindings(locale: string, text: string): string[] {
  if (isPseudo(locale)) return [];
  const out: string[] = glossaryFindings(glossary, locale, text);
  if (/[!！¡]/.test(text)) out.push("no exclamation marks in UI copy (web-client/16-look-states-and-wording.md G19)");
  if (new Intl.Locale(locale).language === "ja") {
    if (/[：；（）]/u.test(text))
      out.push(
        'use the ASCII colon, semicolon and parentheses (": ", "; ", " (…)") in Japanese, not "：", "；", "（）" (web-client/16-look-states-and-wording.md G20)',
      );
    if (text.includes(","))
      out.push('use "、" instead of an ASCII comma in Japanese (web-client/16-look-states-and-wording.md G20)');
  }
  if (isSimplifiedChinese(locale)) out.push(...simplifiedChineseFindings(text));
  if (isTraditionalChinese(locale)) out.push(...traditionalChineseFindings(text));
  if (new Intl.Locale(locale).language === "es") out.push(...spanishFindings(text));
  if (new Intl.Locale(locale).language === "pt") out.push(...portugueseFindings(text));
  if (new Intl.Locale(locale).language === "de") out.push(...germanFindings(text));
  if (new Intl.Locale(locale).language === "fr") out.push(...frenchFindings(text));
  return out;
}

/** The punctuation rules of German (web-client/16-look-states-and-wording.md G20). */
function germanFindings(text: string): string[] {
  if (!text.includes('"')) return [];
  return [
    'use "„…“" (and "‚…‘" inside them) as quotation marks in German, not ASCII double quotation marks (web-client/16-look-states-and-wording.md G20)',
  ];
}

/** A narrow no-break space (U+202F), which French puts before ":", ";", "?" and inside « ». */
const NNBSP = "\u202F";
/** ";" or "?" without a narrow no-break space before it, and ":" followed by a space or the end ("https://" stays). */
const FR_UNSPACED = new RegExp(`(?:^|[^${NNBSP}])(?:[;?]|:(?=\\s|$))`, "mu");
const FR_GUILLEMETS = new RegExp(`«(?!${NNBSP})|(?<!${NNBSP})»`, "u");

/** The punctuation rules of French (web-client/16-look-states-and-wording.md G20). */
function frenchFindings(text: string): string[] {
  const out: string[] = [];
  if (text.includes('"'))
    out.push(
      'use "« … »" as quotation marks in French, not ASCII double quotation marks (web-client/16-look-states-and-wording.md G20)',
    );
  if (FR_UNSPACED.test(text))
    out.push(
      'put a narrow no-break space (U+202F) before ":", ";" and "?" in French (web-client/16-look-states-and-wording.md G20)',
    );
  if (FR_GUILLEMETS.test(text))
    out.push(
      "put a narrow no-break space (U+202F) inside « » in French (web-client/16-look-states-and-wording.md G20)",
    );
  return out;
}

/** The punctuation rules of Brazilian Portuguese (web-client/16-look-states-and-wording.md G20). */
function portugueseFindings(text: string): string[] {
  if (!text.includes('"')) return [];
  return [
    'use "“…”" (and "‘…’" inside them) as quotation marks in Portuguese, not ASCII double quotation marks (web-client/16-look-states-and-wording.md G20)',
  ];
}

/** The punctuation rules of Spanish (web-client/16-look-states-and-wording.md G20). */
function spanishFindings(text: string): string[] {
  const out: string[] = [];
  if ((text.match(/\?/g) ?? []).length !== (text.match(/¿/g) ?? []).length)
    out.push('open each question with "¿" in Spanish (web-client/16-look-states-and-wording.md G20)');
  if (text.includes('"'))
    out.push(
      'use "«…»" (and "“…”" inside them) as quotation marks in Spanish, not ASCII double quotation marks (web-client/16-look-states-and-wording.md G20)',
    );
  return out;
}

/** zh-TW and the tags that resolve to it (ADR-0009 §2). */
function isTraditionalChinese(locale: string): boolean {
  return new Intl.Locale(locale).language === "zh" && !isSimplifiedChinese(locale);
}

/** zh-CN and the tags that resolve to it (ADR-0009 §2). */
function isSimplifiedChinese(locale: string): boolean {
  const { language, script, region } = new Intl.Locale(locale);
  return language === "zh" && script !== "Hant" && !["TW", "HK", "MO"].includes(region ?? "");
}

const HAN = "\\p{Script=Han}";
const ZH_ASCII_PUNCTUATION = new RegExp(`${HAN}[,;:?]|[,;?]${HAN}|${HAN}[()]|[()]${HAN}`, "u");

/** The punctuation rules of Simplified Chinese (web-client/16-look-states-and-wording.md G20). */
function simplifiedChineseFindings(text: string): string[] {
  const out: string[] = [];
  if (ZH_ASCII_PUNCTUATION.test(text))
    out.push(
      'use the full-width "，", "；", "：", "？" and "（）" next to Chinese text in Simplified Chinese (web-client/16-look-states-and-wording.md G20)',
    );
  if (/[「」『』]/u.test(text))
    out.push(
      'use "“…”" and "‘…’" as quotation marks in Simplified Chinese, not "「」" or "『』" (web-client/16-look-states-and-wording.md G20)',
    );
  if (/[\p{Script=Hiragana}\p{Script=Katakana}]/u.test(text))
    out.push("no kana in Simplified Chinese (web-client/16-look-states-and-wording.md G20)");
  return out;
}

/**
 * Characters only Simplified Chinese writes: each one's Traditional form is another code point, and Traditional text
 * never uses it (a converted or mixed-up string). Common UI words only; review catches the rest.
 */
const SIMPLIFIED_ONLY =
  /[们这个说发书设认关账号录图视频网络软据库应该对开从过还进选择删让请时间问题错误页动态码为务当将无与实现点击显户级员复帮览项织线绑权验证读写传输]/u;

/** The punctuation and script rules of Traditional Chinese (web-client/16-look-states-and-wording.md G20). */
function traditionalChineseFindings(text: string): string[] {
  const out: string[] = [];
  if (ZH_ASCII_PUNCTUATION.test(text))
    out.push(
      'use the full-width "，", "；", "：", "？" and "（）" next to Chinese text in Traditional Chinese (web-client/16-look-states-and-wording.md G20)',
    );
  if (/[“”‘’]/u.test(text))
    out.push(
      'use "「…」" and "『…』" as quotation marks in Traditional Chinese, not "“”" or "‘’" (web-client/16-look-states-and-wording.md G20)',
    );
  if (/[\p{Script=Hiragana}\p{Script=Katakana}]/u.test(text))
    out.push("no kana in Traditional Chinese (web-client/16-look-states-and-wording.md G20)");
  const simplified = text.match(SIMPLIFIED_ONLY);
  if (simplified)
    out.push(
      `"${simplified[0]}" is a Simplified Chinese character; write Traditional Chinese (web-client/16-look-states-and-wording.md G20)`,
    );
  return out;
}

/** Checks the source messages, then every translation against them; returns the source ids it checked. */
function checkCatalog(catalog: Catalog, only?: Set<string>, locales?: string[]): string[] {
  if (!catalog[SOURCE]) {
    error(`locales/${SOURCE}: the source locale is missing`);
    return [];
  }
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
      const name = goName(id);
      if (accessors.has(name)) error(`${id} and ${accessors.get(name)} both map to the accessor ${name}`);
      accessors.set(name, id);
      if (only && !only.has(id)) continue;
      if (!KEY.test(key)) error(`${at(SOURCE, ns, key)}: key must match ${KEY}`);
      if (!e.description) warn(`${at(SOURCE, ns, key)}: no description for translators`);
      sourceArgs.set(id, analyze(SOURCE, ns, key, e.defaultMessage));
    }
  }

  for (const [locale, namespaces] of Object.entries(catalog)) {
    if (locale === SOURCE || (locales && !locales.includes(locale))) continue;
    for (const id of sourceArgs.keys()) {
      const [ns, key] = id.split(/:(.*)/s);
      if (isPseudo(locale) && pseudoPart(ns) !== "go") continue;
      if (namespaces[ns]?.[key]) continue;
      if (REQUIRED_LOCALES.includes(locale))
        error(
          `${at(locale, ns, key)}: missing; ${locale} is written in the same change as ${SOURCE}: scripts/i18n.sh set ${id} --${locale} "<text>"`,
        );
      else warn(`${locale}: untranslated ${id}`);
    }
    for (const [ns, entries] of Object.entries(namespaces)) {
      for (const [key, e] of Object.entries(entries)) {
        const id = `${ns}:${key}`;
        if (only && !only.has(id)) continue;
        if (!catalog[SOURCE][ns]?.[key]) {
          error(`${at(locale, ns, key)}: not in the source locale (obsolete); remove it: scripts/i18n.sh rm ${id}`);
          continue;
        }
        const want = sourceArgs.get(id);
        const got = analyze(locale, ns, key, e.defaultMessage);
        if (!want || !got) continue;
        for (const name of compareArgs(`${at(locale, ns, key)}: `, want, got)) {
          if (!isPseudo(locale)) warn(`${at(locale, ns, key)}: drops argument {${name}}`);
        }
      }
    }
  }
  return [...sourceArgs.keys()];
}

/**
 * Warns about keys referenced neither by a Go accessor (a call, a method expression such as
 * `(*i18n.Localizer).ServerX`, or a field chain such as `.L.ServerX` in an html/template file)
 * nor by a TS key literal.
 */
function checkUnused(catalog: Catalog, ids: string[]) {
  const references = new References(Object.keys(catalog[SOURCE]));
  for (const id of ids) {
    if (!references.used(id)) warn(`unused ${id}; refer to it, or remove it: scripts/i18n.sh rm ${id}`);
  }
}

/** Every catalog file is locales/<locale>/<namespace>/<area>.json and holds only the keys of its area (ADR-0081). */
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

export type CheckOptions = {
  /** Check only the messages of these ids (the keys of a change), in every locale. */
  only?: Set<string>;
  /** Of the translations, check only these locales. */
  locales?: string[];
};

/** Runs every check and returns the findings; `options` narrow the messages, never the rules. */
export async function runCheck({ only, locales }: CheckOptions = {}): Promise<Finding[]> {
  findings = [];
  // The client pseudo-locales are generated, uncommitted files that may be missing or stale; leave them out.
  const files = loadCatalogFiles().filter((f) => !isPseudo(f.locale) || (f.ns !== "" && pseudoPart(f.ns) === "go"));
  const catalog = loadCatalog(files);
  checkLayout(files);
  const ids = checkCatalog(catalog, only, locales);
  if (!findings.some((f) => f.level === "error")) {
    for (const f of budgetFindings(catalog, loadBaseline(), only)) error(f);
    checkUnused(catalog, ids);
    await checkGenerated(catalog, files);
  }
  return findings;
}

/** Prints the errors, the warnings `warnings` accepts, and the summary line; returns the number of errors. */
export function printFindings(
  all: Finding[],
  { warnings = () => true, seconds }: { warnings?: (message: string) => boolean; seconds?: number } = {},
) {
  const count = (level: Finding["level"]) => all.filter((f) => f.level === level).length;
  for (const f of all) {
    if (f.level === "error") console.error(`error: ${f.message}`);
    else if (warnings(f.message)) console.warn(`warn:  ${f.message}`);
  }
  console.log(
    `i18n: ${count("error")} error(s), ${count("warn")} warning(s)${seconds === undefined ? "" : ` in ${seconds.toFixed(1)}s`}`,
  );
  return count("error");
}

if (import.meta.main && printFindings(await runCheck()) > 0) process.exit(1);
