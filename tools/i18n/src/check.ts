// SPDX-License-Identifier: AGPL-3.0-only
// Catalog checks run by `scripts/task.sh i18n:check` (ADR-0009 §1), and, for the keys of a change, by
// `scripts/i18n.sh check --changed` and by the commands of cli.ts that write a message.
//   error: ICU syntax error, including a plural / select without `other`
//   error: translation argument unknown to the source, or of a different type
//   error: plural / selectordinal category the locale's CLDR rules do not have
//   error: translation key absent from the source (obsolete)
//   error: rich-text tag, number skeleton / style, or date / time argument (outside the subset Go formats)
//   error: malformed locale directory, namespace, or key; two keys mapping to one accessor
//   error: catalog file outside locales/<locale>/<namespace>/<area>.json, or a key outside its area's file (ADR-0081)
//   error: generated files (pseudo-locales, Go accessors, golden) differ from `scripts/task.sh i18n:gen`
//   error: copy outside the tone and punctuation rules (web-client/16-look-states-and-wording.md G19, G20): an exclamation mark
//          in any non-pseudo locale; a full-width colon, semicolon or parenthesis, or an ASCII comma, in Japanese text
//          (use ":", ";", "()" and "、"); in Simplified Chinese, an ASCII comma, semicolon, colon, question mark or
//          parenthesis next to a Han character, corner brackets 「」『』, or kana; in Traditional Chinese, the same ASCII
//          punctuation, curly quotation marks “”‘’ (use 「」『』), kana, or a character only Simplified Chinese writes;
//          in Spanish, a question mark without its opening "¿", or ASCII double quotation marks (use «…»); in
//          Portuguese, ASCII double quotation marks (use “…”); in German, ASCII double quotation marks (use „…“); in
//          French, ASCII double quotation marks (use « … »), ":", ";" or "?" without the narrow no-break space before
//          it, or « » without the narrow no-break space inside them
//   error: a variant the glossary (tools/i18n/glossary.json, G21) bans in the text of a machine-translated locale
//   error: key of the source missing in ja, which is written in the same change as en (`scripts/i18n.sh set`)
//   error, answer: the copy lint of en and ja (copy.ts): a finding of a blocking rule, or of a waivable rule that
//          tools/i18n/waivers.json does not waive, is an error, a waiver it refuses too (waivers.ts); the rest is
//          printed for the author to answer
//   warn:  untranslated key in a machine-translated locale (falls back to en at runtime)
//   warn:  a machine-translated locale drops a source argument (sometimes legitimate)
//   warn:  key not referenced from code
import { existsSync, readFileSync } from "node:fs";
import {
  areaOf,
  type Catalog,
  type CatalogFile,
  catalogFile,
  formattingLocale,
  isPseudo,
  loadCatalog,
  loadCatalogFiles,
  rel,
  REQUIRED_LOCALES,
  SOURCE,
} from "./catalog";
import { COPY_LOCALES, type CopyFinding, copyFindings, copyText, loadWords, type Word } from "./copy";
import { generate, goName } from "./generate";
import { glossaryFindings, loadGlossary } from "./glossary";
import { type Args, extractArgs, literalText, outsideSharedSubset, parseMessage, pluralCategories } from "./icu";
import { References } from "./refs";
import { loadWaivers, type Waiver, WAIVERS, waiverErrors, waiverKey } from "./waivers";

export type Finding = { level: "error" | "warn" | "answer"; message: string };
let findings: Finding[] = [];
const error = (m: string) => void findings.push({ level: "error", message: m });
const warn = (m: string) => void findings.push({ level: "warn", message: m });
const answer = (m: string) => void findings.push({ level: "answer", message: m });

/** The word decisions and the waivers of the copy lint, and why they could not be read. */
const { words, waivers, unreadable } = (() => {
  const unreadable: string[] = [];
  const read = <T>(load: () => T[]): T[] => {
    try {
      return load();
    } catch (e) {
      unreadable.push((e as Error).message);
      return [];
    }
  };
  return { words: read<Word>(loadWords), waivers: read<Waiver>(loadWaivers), unreadable };
})();

export const NAMESPACE = /^[a-z][a-z0-9]*$/;
/** "locales/<locale>/<namespace>/<area>.json <key>", where a finding about a key points. */
const at = (locale: string, ns: string, key: string): string => `${rel(catalogFile(locale, ns, key))} ${key}`;
export const KEY = /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$/;
const glossary = loadGlossary();

/**
 * The errors of one message on its own (ICU syntax, the shared subset, the copy rules, the plural categories of its
 * locale) and of its arguments against the source message `source`, without the path they are reported under.
 * `dropped` lists the source arguments it leaves out. In en and ja, the errors include the findings of the copy lint
 * that block, and the waivable ones tools/i18n/waivers.json does not waive for the key `key`.
 */
export function messageFindings(
  locale: string,
  ns: string,
  message: string,
  source?: string,
  key = "",
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
    const waived = new Set(waivers.map(waiverKey));
    for (const f of copyOf(locale, message, source)) {
      // A dropped argument is the caller's to refuse (`--drop-args`).
      if (f.tier === "block") error(f.message);
      else if (
        f.tier === "waive" &&
        f.rule !== "placeholder" &&
        !waived.has(waiverKey({ id: `${ns}:${key}`, locale, rule: f.rule }))
      )
        error(waivable(f));
    }
    return { errors: findings.map((f) => f.message), dropped };
  } finally {
    findings = collected;
  }
}

const waivable = (f: CopyFinding): string =>
  `${f.message} (${f.rule}: change the text, or waive it in ${WAIVERS} with the reason)`;

/** The copy findings of an en or ja message; for ja, against the en message `source`. */
function copyOf(locale: string, message: string, source?: string): CopyFinding[] {
  if (!COPY_LOCALES.includes(locale)) return [];
  try {
    const ast = parseMessage(message);
    let en: { text: string; missing: string[] } | undefined;
    if (locale !== SOURCE && source !== undefined) {
      const sourceAst = parseMessage(source);
      const got = extractArgs(ast);
      en = { text: copyText(sourceAst), missing: [...extractArgs(sourceAst).keys()].filter((n) => !got.has(n)) };
    }
    return copyFindings(words, locale, copyText(ast), en);
  } catch {
    // analyze reports the syntax.
    return [];
  }
}

/**
 * The copy lint of every en and ja message (or of the ids `only`), and the waivers against what it finds in all of
 * them, so that a waiver of a message the change did not touch still has to waive something.
 */
function checkCopy(catalog: Catalog, only?: Set<string>, locales?: string[]) {
  for (const message of unreadable) error(message);
  const found = new Set<string>();
  const waived = new Set(waivers.map(waiverKey));
  for (const [ns, entries] of Object.entries(catalog[SOURCE] ?? {})) {
    for (const [key, e] of Object.entries(entries)) {
      const id = `${ns}:${key}`;
      for (const locale of COPY_LOCALES) {
        if (locale !== SOURCE && locales && !locales.includes(locale)) continue;
        const message = locale === SOURCE ? e.defaultMessage : catalog[locale]?.[ns]?.[key]?.defaultMessage;
        if (typeof message !== "string") continue;
        for (const f of copyOf(locale, message, locale === SOURCE ? undefined : e.defaultMessage)) {
          const k = waiverKey({ id, locale, rule: f.rule });
          if (f.tier === "waive") found.add(k);
          if (only && !only.has(id)) continue;
          const where = `${at(locale, ns, key)}: `;
          if (f.tier === "block") error(`${where}${f.message}`);
          else if (f.tier === "answer") answer(`${where}${f.message}`);
          else if (!waived.has(k)) error(`${where}${waivable(f)}`);
        }
      }
    }
  }
  for (const message of waiverErrors(waivers, found)) error(message);
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
    for (const f of outsideSharedSubset(ast)) error(`${where}${f} is outside the Go/TS shared subset`);
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
      const [ns = "", key = ""] = id.split(/:(.*)/s);
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
          // The copy lint holds en and ja to the arguments of the source.
          if (!isPseudo(locale) && !COPY_LOCALES.includes(locale))
            warn(`${at(locale, ns, key)}: drops argument {${name}}`);
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
  const references = new References(Object.keys(catalog[SOURCE] ?? {}));
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
  const generated = await generate(catalog);
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
  only?: Set<string> | undefined;
  /** Of the translations, check only these locales. */
  locales?: string[] | undefined;
};

/** Runs every check and returns the findings; `options` narrow the messages, never the rules. */
export async function runCheck({ only, locales }: CheckOptions = {}): Promise<Finding[]> {
  findings = [];
  const files = loadCatalogFiles();
  const catalog = loadCatalog(files);
  checkLayout(files);
  const ids = checkCatalog(catalog, only, locales);
  if (!findings.some((f) => f.level === "error")) {
    checkUnused(catalog, ids);
    await checkGenerated(catalog, files);
  }
  checkCopy(catalog, only, locales);
  return findings;
}

/** Prints the errors, the answers, the warnings `warnings` accepts, and the summary line; returns the number of errors. */
export function printFindings(
  all: Finding[],
  { warnings = () => true, seconds }: { warnings?: (message: string) => boolean; seconds?: number } = {},
) {
  const count = (level: Finding["level"]) => all.filter((f) => f.level === level).length;
  for (const f of all) {
    if (f.level === "error") console.error(`error: ${f.message}`);
    else if (f.level === "answer") console.warn(`answer: ${f.message}`);
    else if (warnings(f.message)) console.warn(`warn:  ${f.message}`);
  }
  console.log(
    `i18n: ${count("error")} error(s), ${count("answer")} to answer, ${count("warn")} warning(s)${seconds === undefined ? "" : ` in ${seconds.toFixed(1)}s`}`,
  );
  return count("error");
}

if (import.meta.main && printFindings(await runCheck()) > 0) process.exit(1);
