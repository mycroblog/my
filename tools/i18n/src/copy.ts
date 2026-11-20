// SPDX-License-Identifier: AGPL-3.0-only
// The copy lint of the en and ja catalogs (ADR-0169 §5, §8), which check.ts runs on every message of the two.
// Each finding has a tier:
//   block:  fails, and no waiver lets it pass
//   waive:  fails unless tools/i18n/waivers.json waives it with a reason (waivers.ts)
//   answer: printed for the author to answer; never fails
// The rules, by the name a waiver gives:
//   words (block)        a word the copy role's decisions (.claude/knowledge/copy/decisions.md) say to avoid, named
//                        with the word to use; a line "to users" spares the strings only administrators read (ADMIN)
//   next-step (block)    an error (errorOf) whose en or ja text says no next step (hasNextStep), unless the reviewed
//                        list tools/i18n/no-step.json holds it with why no step is possible (check.ts runs it)
//   emoji (block)        an emoji
//   dash (waive)         a dash (‒ – — ― ⸺ ⸻ －, or a spaced hyphen " - ")
//   colon (waive)        a decorative colon: a label (at most four words, or 15 characters in ja) that ends in one
//   kana (waive)         ja: an auxiliary verb in kanji (下さい, て頂く, て見る, て置く, ...), which is written in hiragana
//   formal-noun (answer) ja: a formal noun in kanji after kana (の事, した時, ある物, ...), which is written in hiragana
//   url, code (block)    ja: the URLs, or the `code` spans, differ from the en text
//   placeholder (waive)  ja: an argument of the en text is missing
//   number (answer)      ja: the numbers differ from the en text (en may spell them out: "once", "a week")
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { type MessageFormatElement, TYPE } from "@formatjs/icu-messageformat-parser";
import { repoRoot, SOURCE, toolDir } from "./catalog";

type Tier = "block" | "waive" | "answer";
export type CopyFinding = { tier: Tier; rule: string; message: string };

/** The rules a waiver may name. */
export const WAIVABLE: readonly string[] = ["dash", "colon", "kana", "placeholder"];

/** The locales the copy lint reads: the source and the locale written in the same change. */
export const COPY_LOCALES: readonly string[] = [SOURCE, "ja"];

/** The copy role's decisions, from the repository root; a word line there is a rule of this lint. */
export const DECISIONS = ".claude/knowledge/copy/decisions.md";
const decisionsPath = join(toolDir, "..", "..", DECISIONS);

/**
 * A word decision: in `locale`, write `use` instead of what the `avoid` expressions match. `users`: the decision holds
 * in the strings users read and spares those only administrators read, which may name what such a reader works with.
 */
export type Word = { locale: string; use: string; avoid: RegExp[]; users: boolean };

/** `- <locale>[, to users]: write <term>, avoid `<regexp>`[, ...][; write <term>, avoid ...] — <why> — ...` */
const WORD_LINE = /^- ([a-z]{2})(, to users)?: (write .+?) — /;
const WORD_CLAUSE = /^write (.+?), avoid (`[^`]+`(?:, `[^`]+`)*)$/;

/** Reads the word decisions of `text` (decisions.md); throws on a word line out of shape or a pattern that does not compile. */
export function parseWords(text: string, path = DECISIONS): Word[] {
  const words: Word[] = [];
  text.split("\n").forEach((line, i) => {
    if (!/^- [a-z]{2}(?:, to users)?: /.test(line)) return;
    const shape = new Error(
      `${path}:${i + 1}: a word decision is - <locale>[, to users]: write "<term>", avoid \`<regexp>\`[, \`<regexp>\`][; write ...] — <why> — <from> — <YYYY-MM-DD> — held N / missed N`,
    );
    const m = WORD_LINE.exec(line);
    if (!m) throw shape;
    const [, locale = "", users, clauses = ""] = m;
    for (const clause of clauses.split(/; (?=write )/)) {
      const c = WORD_CLAUSE.exec(clause);
      if (!c) throw shape;
      const [, use = "", avoid = ""] = c;
      const patterns = [...avoid.matchAll(/`([^`]+)`/g)].map(([, p = ""]) => {
        try {
          return new RegExp(p, "mu");
        } catch (e) {
          throw new Error(`${path}:${i + 1}: ${(e as Error).message}`, { cause: e });
        }
      });
      words.push({ locale, use, avoid: patterns, users: users !== undefined });
    }
  });
  return words;
}

/**
 * A string only administrators read: its en description says so in a sentence of its own, "Read by administrators."
 * (or by moderators, staff, or other servers' operators).
 */
export const ADMIN = /\bRead by (?:administrators|moderators|staff|other servers' operators)\./;

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
/** The characters of a text as a reader counts them: its grapheme clusters, an emoji with its modifiers as one. */
const graphemes = (text: string): string[] =>
  Array.from(new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(text), (s) => s.segment);

/** A line that ends in a colon; the colon is decorative when the line is a label, not a sentence that leads into a list. */
const COLON_LINE = /^(.*)[:：][ \t]*$/gmu;
function isLabel(locale: string, line: string): boolean {
  const text = line.replace(/\{[^{}]*\}/g, "x").trim();
  return locale === "ja" ? graphemes(text.replace(/\s/g, "")).length <= 15 : text.split(/\s+/).length <= 4;
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

/**
 * The error a key belongs to, for the next-step rule: a key with a segment `error` or `errors` (server:errors.*,
 * payments.error.*), or a key of a web package whose last segment is or ends in `failed` / `failed_more`. One error
 * is the key and its `_detail`, `group` the key without `_detail`. Not an error: a `title` (the heading over one) and
 * notifications.kind.* (a notification that something failed, not an error of the reader's own action). `retry`:
 * a failure of the client, which G18 shows with the retry button that is its next step.
 */
export function errorOf(ns: string, key: string): { group: string; retry: boolean } | undefined {
  const segments = key.split(".");
  const last = segments.at(-1) ?? "";
  if (last === "title" || key.startsWith("notifications.kind.")) return undefined;
  const group = `${ns}:${key.replace(/[._]detail$/, "")}`;
  if (segments.some((s) => s === "error" || s === "errors")) return { group, retry: false };
  if (ns !== "server" && /(?:^|_)failed(?:_more)?$/.test(last)) return { group, retry: true };
  return undefined;
}

/** en: the verbs a next step starts with. */
const STEP_VERBS =
  "add|allow|approve|ask|cancel|change|check|choose|close|configure|confirm|contact|delete|disable|edit|enable|enter|export|follow|give|go|keep|leave|link|make|move|open|pick|post|raise|record|reject|reload|remove|request|retire|revoke|save|schedule|select|send|set|shorten|sign|start|turn|try|unblock|unfollow|unlink|unmute|unpin|update|upload|use|verify|wait|write";
/**
 * A next step. ja: a sentence that asks with ください; as a field error does, what is needed or the limit (必要です,
 * までです); what the reader can do (できます, られます); or that nothing needs doing (かまいません). en: a sentence, a
 * branch, or a clause after a comma or a colon that starts with an imperative of STEP_VERBS; the value the server
 * needs (must, at most, need); or what the reader can do (you can, your account can, available again).
 */
const STEP: Record<string, RegExp> = {
  ja: /ください(?:[。)」]|$)|必要です|までです|(?<!(?:だけ|のみ)[^。\n]*)(?:でき|られ)ます(?:。|$)|かまいません/mu,
  en: new RegExp(
    `(?:^|[.?:;,]\\s+|\\n)(?:Please\\s+|then\\s+|or\\s+)?(?:${STEP_VERBS})\\b|\\b(?:must|at most|needs?|available again)\\b|\\b(?:you|your account) can (?:still |again )?(?!not\\b|only\\b|no\\b)`,
    "imu",
  ),
};
/** G18: the failure of a part of the client, which the retry button follows. */
const RETRY: Record<string, RegExp> = {
  ja: /ませんでした(?:。|$)/mu,
  en: /\b(?:could not be|did not finish)\b/u,
};

/** Whether the text of an error says what to do next, or, with `retry`, is the G18 failure the retry button follows. */
export function hasNextStep(locale: string, text: string, retry = false): boolean {
  return Boolean(STEP[locale]?.test(text) || (retry && RETRY[locale]?.test(text)));
}

/**
 * The reviewed list of the errors with no possible step: `{ "<why no step>": ["<namespace>:<error>", ...] }`, where
 * an entry is a whole error or one branch of a message of it (`<namespace>:<key>#<branch>`, branchTexts).
 */
export const NO_STEP = "tools/i18n/no-step.json";
/** In the repository the tool reads, as the list is about its catalogs (I18N_TOOL_ROOT). */
const noStepPath = join(repoRoot, NO_STEP);
/** The shortest reason, in characters. */
const MIN_NO_STEP_REASON = 40;

/** The errors of the list (`text`) by id, with their reason; throws on a list out of shape. */
export function parseNoStep(text: string): Map<string, string> {
  const data: unknown = JSON.parse(text);
  const shape = `${NO_STEP}: an object of {"<why no step is possible>": ["<namespace>:<error>[#<branch>]", ...]}`;
  if (typeof data !== "object" || data === null || Array.isArray(data)) throw new Error(shape);
  const out = new Map<string, string>();
  for (const [reason, ids] of Object.entries(data)) {
    if (!Array.isArray(ids) || !ids.every((id) => typeof id === "string")) throw new Error(shape);
    if (reason.trim().length < MIN_NO_STEP_REASON)
      throw new Error(
        `${NO_STEP}: "${reason}": say in at least ${MIN_NO_STEP_REASON} characters why no step is possible`,
      );
    for (const id of ids) {
      if (out.has(id)) throw new Error(`${NO_STEP}: ${id} is listed twice; keep one`);
      out.set(id, reason);
    }
  }
  return out;
}

export const loadNoStep = (path: string = noStepPath): Map<string, string> =>
  existsSync(path) ? parseNoStep(readFileSync(path, "utf8")) : new Map<string, string>();

/**
 * Renames the ids of the reviewed list (`scripts/i18n.sh mv`), and drops one whose new key is no error (errorOf);
 * returns whether it changed.
 */
export function renameNoStep(renames: Map<string, string>, path: string = noStepPath): boolean {
  if (!existsSync(path)) return false;
  const data = JSON.parse(readFileSync(path, "utf8")) as Record<string, string[]>;
  const base = (id: string) => id.split("#")[0] ?? id;
  if (!Object.values(data).some((ids) => ids.some((id) => renames.has(base(id))))) return false;
  const renamed = (id: string): string[] => {
    const [from = "", branch] = id.split("#");
    const moved = renames.get(from);
    if (moved === undefined) return [id];
    const to = branch === undefined ? moved : `${moved}#${branch}`;
    const [ns = "", key = ""] = moved.split(/:(.*)/s);
    return errorOf(ns, key) ? [to] : [];
  };
  const next = Object.fromEntries(
    Object.entries(data)
      .map(([reason, ids]) => [reason, ids.flatMap(renamed)] as const)
      .filter(([, ids]) => ids.length),
  );
  writeFileSync(path, `${JSON.stringify(next, null, 2)}\n`);
  return true;
}

/**
 * The texts a message can show, by branch: one per choice of each select and plural, with the text around it. The
 * id of a branch is its option keys, outer first, joined by "/" (`exists`, `name/one`); a message without a select
 * or a plural has the one branch "". Arguments are kept as `{name}`, as in copyText.
 */
export function branchTexts(ast: MessageFormatElement[]): Map<string, string> {
  type Branch = { keys: string[]; text: string };
  const join = (heads: Branch[], tails: Branch[]): Branch[] =>
    heads.flatMap((h) => tails.map((t) => ({ keys: [...h.keys, ...t.keys], text: h.text + t.text })));
  const walk = (els: MessageFormatElement[]): Branch[] => {
    let out: Branch[] = [{ keys: [], text: "" }];
    for (const el of els) {
      switch (el.type) {
        case TYPE.literal:
          out = join(out, [{ keys: [], text: el.value }]);
          break;
        case TYPE.argument:
        case TYPE.number:
        case TYPE.date:
        case TYPE.time:
          out = join(out, [{ keys: [], text: `{${el.value}}` }]);
          break;
        case TYPE.pound:
          out = join(out, [{ keys: [], text: "#" }]);
          break;
        case TYPE.plural:
        case TYPE.select:
          out = join(
            out,
            Object.entries(el.options).flatMap(([key, o]) =>
              walk(o.value).map((b) => ({ keys: [key, ...b.keys], text: b.text })),
            ),
          );
          break;
        case TYPE.tag:
          out = join(out, walk(el.children));
          break;
      }
    }
    return out;
  };
  return new Map(walk(ast).map((b) => [b.keys.join("/"), b.text]));
}

/**
 * The messages of one error (errorOf) in en and ja: each message's key and its texts by branch (branchTexts);
 * whether a key of it is among the checked ones.
 */
export type ErrorTexts = {
  texts: { locale: string; key: string; branches: Map<string, string> }[];
  retry: boolean;
  checked: boolean;
};

/**
 * The branches of one error that say no next step, as `<ns>:<key>#<branch>` (or the group id for an error without
 * branches), each with the locales it lacks one in. A branch has its step when the branch says one, or when another
 * message of the error does in its branch of the same id, or as a whole when it has no branches (a title over the
 * detail's branches).
 */
function stepless(group: string, { texts, retry }: ErrorTexts): Map<string, string[]> {
  const ns = group.split(":")[0] ?? "";
  const out = new Map<string, string[]>();
  for (const locale of COPY_LOCALES) {
    const msgs = texts.filter((t) => t.locale === locale);
    if (!msgs.length) continue;
    const step = (text: string | undefined) => text !== undefined && hasNextStep(locale, text, retry);
    const ids = new Set(msgs.flatMap((m) => [...m.branches.keys()].filter((b) => b !== "")));
    const lack = (id: string) => void out.set(id, [...(out.get(id) ?? []), locale]);
    if (!ids.size) {
      if (!msgs.some((m) => step(m.branches.get("")))) lack(group);
      continue;
    }
    for (const branch of ids) {
      if (msgs.some((m) => step(m.branches.get(branch)) || (m.branches.size === 1 && step(m.branches.get("")))))
        continue;
      const owner = msgs.find((m) => m.branches.has(branch));
      lack(`${ns}:${owner?.key ?? ""}#${branch}`);
    }
  }
  return out;
}

/**
 * The findings of the next-step rule: an error, or a branch of one (`<ns>:<key>#<branch>`), whose en or ja text has
 * no next step, unless the reviewed list `noStep` holds it (a whole error holds all its branches); and an entry of
 * the list that names nothing that lacks a step: an error or a branch that has its step now, or none of the catalogs.
 */
export function nextStepErrors(errors: Map<string, ErrorTexts>, noStep: Map<string, string>): string[] {
  const out: string[] = [];
  const lacking = new Set<string>();
  for (const [group, error] of errors) {
    const missing = stepless(group, error);
    for (const [id, locales] of missing) {
      lacking.add(id);
      if (!error.checked || noStep.has(group) || noStep.has(id)) continue;
      const what = id === group ? "the error" : "this branch";
      out.push(
        `${id}: ${what} says no next step in ${locales.join(" and ")}; end it with what the reader can do (en: an imperative such as "Try again."; ja: 〜してください), or, when no step is possible, add it to ${NO_STEP} under the reason`,
      );
    }
    const both = COPY_LOCALES.every((l) => error.texts.some((t) => t.locale === l));
    if (missing.size) lacking.add(group);
    else if (both && noStep.has(group))
      out.push(`${NO_STEP}: ${group} says its next step now; remove it from the list`);
  }
  for (const id of noStep.keys()) {
    if (id.includes("#")) {
      if (!lacking.has(id))
        out.push(
          `${NO_STEP}: ${id} is no branch of an error that lacks a next step (it has one now, or is gone); remove it`,
        );
    } else if (!errors.has(id)) out.push(`${NO_STEP}: ${id} is not an error of the catalogs (errorOf); remove it`);
  }
  return out;
}

const same = (a: string[], b: string[]): boolean => [...a].sort().join("\0") === [...b].sort().join("\0");
const list = (items: string[]): string => (items.length ? items.join(", ") : "none");

/**
 * The copy findings of one message: `text` is its copyText; for a ja message, `source` is the copyText of the en
 * message and `missing` the en arguments it does not use; `adminOnly`, that only administrators read it (ADMIN).
 */
export function copyFindings(
  words: Word[],
  locale: string,
  text: string,
  source?: { text: string; missing: string[] },
  adminOnly = false,
): CopyFinding[] {
  if (!COPY_LOCALES.includes(locale)) return [];
  const out: CopyFinding[] = [];
  const add = (tier: Tier, rule: string, message: string) => void out.push({ tier, rule, message });
  // The words of the text, not the names of its arguments.
  const prose = text.replace(/\{[^{}]*\}/g, "{}");
  for (const word of words) {
    if (word.locale !== locale || (word.users && adminOnly)) continue;
    for (const avoid of word.avoid) {
      const found = avoid.exec(prose);
      if (found) add("block", "words", `"${found[0]}" is a word to avoid; use ${word.use} (${DECISIONS})`);
    }
  }
  const emoji = graphemes(text).find((c) => EMOJI.test(c) && !NOT_EMOJI.has(c.replace(/\uFE0F/gu, "")));
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
