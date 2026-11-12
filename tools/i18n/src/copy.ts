// SPDX-License-Identifier: AGPL-3.0-only
// The copy lint of the en and ja catalogs (ADR-0169 §5, §8), which check.ts runs on every message of the two.
// Each finding has a tier:
//   block:  fails, and no waiver lets it pass
//   waive:  fails unless tools/i18n/waivers.json waives it with a reason (waivers.ts)
//   answer: printed for the author to answer; never fails
// The rules, by the name a waiver gives:
//   words (block)        a word the copy role's decisions (.claude/knowledge/copy/decisions.md) say to avoid
//   emoji (block)        an emoji
//   dash (waive)         a dash (‒ – — ― ⸺ ⸻ －, or a spaced hyphen " - ")
//   colon (waive)        a decorative colon: a label (at most four words, or 15 characters in ja) that ends in one
//   kana (waive)         ja: an auxiliary verb in kanji (下さい, て頂く, て見る, て置く, ...), which is written in hiragana
//   formal-noun (answer) ja: a formal noun in kanji after kana (の事, した時, ある物, ...), which is written in hiragana
//   url, code (block)    ja: the URLs, or the `code` spans, differ from the en text
//   placeholder (waive)  ja: an argument of the en text is missing
//   number (answer)      ja: the numbers differ from the en text (en may spell them out: "once", "a week")
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { type MessageFormatElement, TYPE } from "@formatjs/icu-messageformat-parser";
import { SOURCE, toolDir } from "./catalog";

type Tier = "block" | "waive" | "answer";
export type CopyFinding = { tier: Tier; rule: string; message: string };

/** The rules a waiver may name. */
export const WAIVABLE: readonly string[] = ["dash", "colon", "kana", "placeholder"];

/** The locales the copy lint reads: the source and the locale written in the same change. */
export const COPY_LOCALES: readonly string[] = [SOURCE, "ja"];

/** The copy role's decisions, from the repository root; a word line there is a rule of this lint. */
export const DECISIONS = ".claude/knowledge/copy/decisions.md";
const decisionsPath = join(toolDir, "..", "..", DECISIONS);

/** A word decision: in `locale`, write `use` instead of what the `avoid` expressions match. */
export type Word = { locale: string; use: string; avoid: RegExp[] };

/** `- <locale>: write <term>, avoid `<regexp>`[, `<regexp>`...] — <why> — ...` */
const WORD_LINE = /^- ([a-z]{2}): write (.+?), avoid (`[^`]+`(?:, `[^`]+`)*) — /;

/** Reads the word decisions of `text` (decisions.md); throws on a word line out of shape or a pattern that does not compile. */
export function parseWords(text: string, path = DECISIONS): Word[] {
  const words: Word[] = [];
  text.split("\n").forEach((line, i) => {
    if (!/^- [a-z]{2}: /.test(line)) return;
    const m = WORD_LINE.exec(line);
    if (!m)
      throw new Error(
        `${path}:${i + 1}: a word decision is - <locale>: write "<term>", avoid \`<regexp>\`[, \`<regexp>\`] — <why> — <from> — <YYYY-MM-DD> — held N / missed N`,
      );
    const [, locale = "", use = "", avoid = ""] = m;
    const patterns = [...avoid.matchAll(/`([^`]+)`/g)].map(([, p = ""]) => {
      try {
        return new RegExp(p, "mu");
      } catch (e) {
        throw new Error(`${path}:${i + 1}: ${(e as Error).message}`);
      }
    });
    words.push({ locale, use, avoid: patterns });
  });
  return words;
}

export const loadWords = (path: string = decisionsPath): Word[] => parseWords(readFileSync(path, "utf8"));

/** The text of a message with its arguments kept as `{name}` and its plural / select branches on lines of their own. */
export function copyText(ast: MessageFormatElement[]): string {
  let out = "";
  for (const el of ast) {
    switch (el.type) {
      case TYPE.literal:
        out += el.value;
        break;
      case TYPE.argument:
      case TYPE.number:
      case TYPE.date:
      case TYPE.time:
        out += `{${el.value}}`;
        break;
      case TYPE.pound:
        out += "#";
        break;
      case TYPE.plural:
      case TYPE.select:
        for (const o of Object.values(el.options)) out += `\n${copyText(o.value)}`;
        break;
      case TYPE.tag:
        out += copyText(el.children);
        break;
    }
  }
  return out;
}

const EMOJI = /\p{Extended_Pictographic}/u;
/** Characters Extended_Pictographic holds that are not emoji in UI copy. */
const NOT_EMOJI = new Set(["©", "®", "™"]);
const DASH = /[‒-―⸺⸻－]|(?<=\S) - (?=\S)/u;

/** A line that ends in a colon; the colon is decorative when the line is a label, not a sentence that leads into a list. */
const COLON_LINE = /^(.*)[:：][ \t]*$/gmu;
function isLabel(locale: string, line: string): boolean {
  const text = line.replace(/\{[^{}]*\}/g, "x").trim();
  return locale === "ja" ? [...text.replace(/\s/g, "")].length <= 15 : text.split(/\s+/).length <= 4;
}

/** Auxiliary verbs, which are written in hiragana, and the kanji spellings that give them away. */
const AUX_VERBS: [RegExp, string][] = [
  [/下さ[いっるれ]/u, "ください"],
  [/(?:[てで]|(?<=\p{Script=Han}))頂[かきくけこい]/u, "いただく"],
  [/て見[たてるれよま]/u, "みる"],
  [/て置[かきくけこい]/u, "おく"],
  [/て居[たてるれよま]/u, "いる"],
  [/て行[かきくけこ]/u, "いく"],
  [/て来[たてるれよま]/u, "くる"],
  [/て仕舞/u, "しまう"],
  [/て貰[わいうえおっ]/u, "もらう"],
  [/て欲し/u, "ほしい"],
  [/[てで]有[るりっ]/u, "ある"],
];

/** Formal nouns after hiragana; no kanji may follow, or it is a word of its own (事前, 時間). */
const FORMAL_NOUNS: [RegExp, string][] = [
  [/(?<=\p{Script=Hiragana})事(?!\p{Script=Han})/u, "こと"],
  [/(?<=\p{Script=Hiragana})時(?!\p{Script=Han})/u, "とき"],
  [/(?<=\p{Script=Hiragana})物(?!\p{Script=Han})/u, "もの"],
  [/(?<=\p{Script=Hiragana})所(?!\p{Script=Han})/u, "ところ"],
  [/(?<=\p{Script=Hiragana})為(?!\p{Script=Han})/u, "ため"],
  [/(?<=\p{Script=Hiragana})訳(?!\p{Script=Han})/u, "わけ"],
  [/(?<=\p{Script=Hiragana})筈/u, "はず"],
  [/(?<=\p{Script=Hiragana})程(?!\p{Script=Han})/u, "ほど"],
  [/(?<=\p{Script=Hiragana})様[なにで]/u, "よう"],
];

const URL = /https?:\/\/[^\s"'<>()（）「」、。]+?(?=[.,;:!?]?(?:[\s"'<>()（）「」、。]|$))/gu;
const CODE = /`[^`\n]+`/g;
/** A number, with its thousands separators, or a ja number of 万 (10,000). */
const NUMBER = /(\d{1,3}(?:,\d{3})+(?!\d)|\d+)(?:\s*(万))?/gu;
/** The numbers en spells out, so that "one" or "once" is the same number as a ja "1". */
const NUMBER_WORDS: Record<string, string> = {
  zero: "0",
  one: "1",
  once: "1",
  two: "2",
  twice: "2",
  three: "3",
  four: "4",
  five: "5",
  six: "6",
  seven: "7",
  eight: "8",
  nine: "9",
  ten: "10",
};
const SPELLED = new RegExp(`\\b(?:${Object.keys(NUMBER_WORDS).join("|")})\\b`, "gi");

/** The distinct numbers written in digits in a text, without its arguments and URLs. */
function digitsOf(text: string): Set<string> {
  const plain = text.replace(/\{[^{}]*\}/g, " ").replace(URL, " ");
  return new Set(
    [...plain.matchAll(NUMBER)].map(([, n = "", man]) => String(Number(n.replaceAll(",", "")) * (man ? 10000 : 1))),
  );
}

/** The numbers of the en text that the ja text lacks, and the ja numbers the en text has neither in digits nor in words. */
function numberMismatch(ja: string, en: string): { lacks: string[]; adds: string[] } {
  const want = digitsOf(en);
  const got = digitsOf(ja);
  const spelled = new Set((en.match(SPELLED) ?? []).map((w) => NUMBER_WORDS[w.toLowerCase()]));
  return {
    lacks: [...want].filter((n) => !got.has(n)),
    adds: [...got].filter((n) => !want.has(n) && !spelled.has(n)),
  };
}

const same = (a: string[], b: string[]): boolean => [...a].sort().join("\0") === [...b].sort().join("\0");
const list = (items: string[]): string => (items.length ? items.join(", ") : "none");

/**
 * The copy findings of one message: `text` is its copyText; for a ja message, `source` is the copyText of the en
 * message and `missing` the en arguments it does not use.
 */
export function copyFindings(
  words: Word[],
  locale: string,
  text: string,
  source?: { text: string; missing: string[] },
): CopyFinding[] {
  if (!COPY_LOCALES.includes(locale)) return [];
  const out: CopyFinding[] = [];
  const add = (tier: Tier, rule: string, message: string) => void out.push({ tier, rule, message });
  for (const word of words) {
    if (word.locale !== locale) continue;
    for (const avoid of word.avoid) {
      const found = avoid.exec(text);
      if (found) add("block", "words", `"${found[0]}" is a word to avoid; use ${word.use} (${DECISIONS})`);
    }
  }
  const emoji = [...text].find((c) => EMOJI.test(c) && !NOT_EMOJI.has(c));
  if (emoji) add("block", "emoji", `no emoji in UI copy ("${emoji}")`);
  const dash = DASH.exec(text);
  if (dash)
    add("waive", "dash", `no dashes in UI copy ("${dash[0].trim()}"): end the sentence, or use a comma or a colon`);
  for (const [, line = ""] of text.matchAll(COLON_LINE)) {
    if (isLabel(locale, line))
      add("waive", "colon", `no colon after a label ("${line.trim()}:"): the layout shows that a label is a label`);
  }
  if (locale !== "ja") return out;
  for (const [re, kana] of AUX_VERBS) {
    const found = re.exec(text);
    if (found) add("waive", "kana", `write the auxiliary verb in hiragana: "${found[0]}" is ${kana}`);
  }
  for (const [re, kana] of FORMAL_NOUNS) {
    const found = re.exec(text);
    if (found)
      add("answer", "formal-noun", `is "${found[0]}" the formal noun ${kana}? A formal noun is written in hiragana`);
  }
  if (!source) return out;
  for (const [rule, re, what] of [
    ["url", URL, "URLs"],
    ["code", CODE, "code spans"],
  ] as const) {
    const got = [...(text.match(re) ?? [])];
    const want = [...(source.text.match(re) ?? [])];
    if (!same(got, want)) add("block", rule, `the ${what} differ from the en text: ${list(got)}; en has ${list(want)}`);
  }
  const { lacks, adds } = numberMismatch(text, source.text);
  if (lacks.length || adds.length)
    add(
      "answer",
      "number",
      `do the numbers say what the en text says? ${[lacks.length ? `en has ${list(lacks)}, ja does not` : "", adds.length ? `ja has ${list(adds)}, en does not` : ""].filter(Boolean).join("; ")}`,
    );
  for (const name of source.missing) add("waive", "placeholder", `drops the argument {${name}} of the en text`);
  return out;
}
