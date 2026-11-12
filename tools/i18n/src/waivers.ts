// SPDX-License-Identifier: AGPL-3.0-only
// The waivers of the copy lint (ADR-0169 §5): tools/i18n/waivers.json lists each finding of a waivable rule
// (copy.ts) that a message keeps, with the reason. check.ts refuses a waiver whose reason is shorter than
// MIN_REASON or boilerplate, and one that waives nothing; run as a script (`scripts/task.sh i18n:waivers`), this
// refuses more than CAP waivers added since the merge base with a ref, the waivers of one change.
//
//   bun src/waivers.ts [BASE]    BASE defaults to origin/redesign-web
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { repoRoot, toolDir } from "./catalog";
import { COPY_LOCALES, WAIVABLE } from "./copy";

/** One waiver: the message (`<namespace>:<key>` in `locale`), the rule it keeps, and why. */
export type Waiver = { id: string; locale: string; rule: string; reason: string };

export const WAIVERS = "tools/i18n/waivers.json";
const waiversPath = join(toolDir, "waivers.json");
/** The shortest reason, in characters. */
const MIN_REASON = 40;
/** The most waivers one change may add; more is itself a finding (ADR-0169 §5). */
export const CAP = 3;

export const waiverKey = (w: { id: string; locale: string; rule: string }): string => `${w.id} ${w.locale} ${w.rule}`;

function parseWaivers(text: string): Waiver[] {
  const data: unknown = JSON.parse(text);
  if (!Array.isArray(data)) throw new Error(`${WAIVERS}: a list of waivers`);
  return data as Waiver[];
}

export const loadWaivers = (path: string = waiversPath): Waiver[] =>
  existsSync(path) ? parseWaivers(readFileSync(path, "utf8")) : [];

/** Phrases that say a waiver is wanted, not why; a reason made of little else is boilerplate. */
const STOCK = [
  "false positive",
  "as intended",
  "as designed",
  "by design",
  "intentional",
  "intentionally",
  "on purpose",
  "not an issue",
  "not a problem",
  "not a bug",
  "needed",
  "required",
  "necessary",
  "wontfix",
  "won't fix",
  "ignore",
  "ignored",
  "todo",
  "tbd",
  "fixme",
  "n/a",
  "ok",
  "okay",
  "fine",
  "see above",
  "see below",
  "same as above",
  "for now",
  "temporary",
  "legacy",
  "waived",
  "waive",
  "waiver",
  "exception",
  "the lint is wrong",
  "the rule does not apply",
  "this is fine",
  "this is correct",
  "correct",
  "expected",
];
const STOCK_RE = new RegExp(
  `(?<![\\p{L}])(?:${STOCK.map((s) => s.replace(/[.*+?^${}()|[\]\\/]/g, "\\$&")).join("|")})(?![\\p{L}])`,
  "gu",
);
/** The fewest distinct words, beyond the stock phrases and short words, a reason says. */
const MIN_WORDS = 6;

/** Why `reason` is refused, or undefined when it says why the message keeps what the rule finds. */
export function reasonProblem(reason: string): string | undefined {
  const r = reason.trim();
  if (r.length < MIN_REASON)
    return `the reason has ${r.length} characters; say in at least ${MIN_REASON} why this message keeps it`;
  const rest = r.toLowerCase().replace(STOCK_RE, " ");
  const words = new Set(rest.split(/[^\p{L}\p{N}']+/u).filter((w) => w.length > 2));
  if (words.size < MIN_WORDS)
    return "the reason is boilerplate; say what about this message the rule does not fit, and why the text cannot change";
  return undefined;
}

/**
 * The errors of the waivers against the keys of the waivable findings they may waive (waiverKey): a waiver out of
 * shape, of a rule that is not waivable, given twice, with a refused reason or the reason of another waiver, or that
 * waives nothing.
 */
export function waiverErrors(waivers: Waiver[], found: Set<string>): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  const reasons = new Map<string, string>();
  for (const w of waivers) {
    const where = `${WAIVERS} ${w?.id} ${w?.locale} ${w?.rule}`;
    if (
      typeof w !== "object" ||
      w === null ||
      Object.keys(w).sort().join(",") !== "id,locale,reason,rule" ||
      ![w.id, w.locale, w.rule, w.reason].every((v) => typeof v === "string")
    ) {
      out.push(`${WAIVERS}: a waiver is {"id": "<namespace>:<key>", "locale", "rule", "reason"}: ${JSON.stringify(w)}`);
      continue;
    }
    if (!COPY_LOCALES.includes(w.locale)) out.push(`${where}: the copy lint reads ${COPY_LOCALES.join(" and ")} only`);
    if (!WAIVABLE.includes(w.rule))
      out.push(
        `${where}: ${w.rule} is not waivable; the waivable rules are ${WAIVABLE.join(", ")} (tools/i18n/src/copy.ts)`,
      );
    const key = waiverKey(w);
    if (seen.has(key)) out.push(`${where}: given twice; keep one`);
    seen.add(key);
    const problem = reasonProblem(w.reason);
    if (problem) out.push(`${where}: ${problem}`);
    const same = reasons.get(w.reason.trim().toLowerCase());
    if (same) out.push(`${where}: the same reason as ${same} is boilerplate; say why this message keeps it`);
    reasons.set(w.reason.trim().toLowerCase(), `${w.id} ${w.locale} ${w.rule}`);
    if (WAIVABLE.includes(w.rule) && !found.has(key))
      out.push(`${where}: waives nothing (the message no longer has the finding); remove it`);
  }
  return out;
}

/** The waivers of `now` that `before` does not have, and the refusal when they are more than CAP. */
export function addedWaivers(now: Waiver[], before: Waiver[]): { added: Waiver[]; refusal?: string } {
  const old = new Set(before.map(waiverKey));
  const added = now.filter((w) => !old.has(waiverKey(w)));
  if (added.length <= CAP) return { added };
  return {
    added,
    refusal: `${added.length} waivers in one change, more than ${CAP}: fix the copy, or, when a rule keeps missing, ask the decider to change it or to record a decision (ADR-0169 §4, §5)`,
  };
}

const git = (...args: string[]) => {
  const p = Bun.spawnSync(["git", ...args], { cwd: repoRoot, stderr: "pipe" });
  return { ok: p.exitCode === 0, out: p.stdout.toString() };
};

if (import.meta.main) {
  const base = process.argv[2] || "origin/redesign-web";
  const mergeBase = git("merge-base", base, "HEAD");
  if (!mergeBase.ok) {
    console.error(
      `i18n:waivers: ${base} is not a commit here; pass the base (scripts/task.sh i18n:waivers BASE=<ref>)`,
    );
    process.exit(2);
  }
  const before = git("show", `${mergeBase.out.trim()}:${WAIVERS}`);
  const { added, refusal } = addedWaivers(loadWaivers(), before.ok ? parseWaivers(before.out) : []);
  console.log(`i18n:waivers: ${added.length} waiver(s) added since ${base}, at most ${CAP}`);
  if (refusal) {
    for (const w of added) console.error(`  ${waiverKey(w)}`);
    console.error(`i18n:waivers: ${refusal}`);
    process.exit(1);
  }
}
