// SPDX-License-Identifier: AGPL-3.0-only
// The length budget of the explanatory texts of the clients: a message of a client* namespace whose key ends in
// "description", "hint", "help" or "note" is one short sentence. The texts that were longer when the rule arrived are
// listed, with their lengths, in tools/i18n/helper-text-baseline.json, which only shrinks: a listed text may not grow,
// and an entry whose text now fits or is gone fails the check until it is removed.
import { type MessageFormatElement, TYPE } from "@formatjs/icu-messageformat-parser";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import {
  type Catalog,
  catalogFile,
  isClientNamespace,
  rel,
  repoRoot,
  SOURCE,
  toJSON,
  writeFileAtomic,
} from "./catalog";
import { parseMessage } from "./icu";

const HELPER_KEY = /(description|hint|help|note)$/;
/** The longest a helper text may be, per locale the budget covers: words where words are spaced, else characters. */
export const HELPER_LIMITS: Record<string, { max: number; unit: "words" | "characters" }> = {
  [SOURCE]: { max: 20, unit: "words" },
  ja: { max: 50, unit: "characters" },
};
const ADVICE = "say it in the label, or move the explanation to the documentation";

export const isHelperText = (ns: string, key: string): boolean => isClientNamespace(ns) && HELPER_KEY.test(key);

/** The text a reader sees at most: the longest branch of each plural / select, an argument as one word. */
function longestText(ast: MessageFormatElement[]): string {
  let out = "";
  for (const el of ast) {
    if (el.type === TYPE.literal) out += el.value;
    else if (el.type === TYPE.plural || el.type === TYPE.select) {
      out += Object.values(el.options)
        .map((o) => longestText(o.value))
        .reduce((a, b) => (b.length > a.length ? b : a), "");
    } else if (el.type === TYPE.tag) out += longestText(el.children);
    else out += "x";
  }
  return out;
}

/** The length of a message in words and characters; zeros when it does not parse (check.ts reports that). */
export function measure(message: string): { words: number; characters: number } {
  let text = message;
  try {
    text = longestText(parseMessage(message));
  } catch {
    return { words: 0, characters: 0 };
  }
  text = text.trim();
  return { words: text.split(/\s+/).filter(Boolean).length, characters: [...text].length };
}

const lengthIn = (locale: string, message: string | undefined): number =>
  message === undefined ? 0 : measure(message)[HELPER_LIMITS[locale].unit];

/** id -> the length each budgeted locale's text had when it was listed. */
export type Baseline = Record<string, Record<string, number>>;
export const baselinePath = join(repoRoot, "tools", "i18n", "helper-text-baseline.json");
export const loadBaseline = (): Baseline =>
  existsSync(baselinePath) ? JSON.parse(readFileSync(baselinePath, "utf8")) : {};

/** The findings of one helper text against the limits, or against its baseline entry when it has one. */
export function helperFindings(
  ns: string,
  key: string,
  texts: Record<string, string | undefined>,
  allowed: Record<string, number> | undefined,
): string[] {
  if (!isHelperText(ns, key)) return [];
  const out: string[] = [];
  for (const [locale, { max, unit }] of Object.entries(HELPER_LIMITS)) {
    const got = lengthIn(locale, texts[locale]);
    const where = `${rel(catalogFile(locale, ns, key))} ${key}`;
    if (got <= max) continue;
    if (!allowed)
      out.push(`${where}: helper text is ${got} ${unit}, the limit is ${max} (one short sentence); ${ADVICE}`);
    else if (got > Math.max(max, allowed[locale] ?? 0))
      out.push(
        `${where}: helper text grew to ${got} ${unit}; the limit is ${max} and its entry in ${rel(baselinePath)} allows ${allowed[locale] ?? max}; shorten it: ${ADVICE}`,
      );
  }
  return out;
}

const textsOf = (catalog: Catalog, ns: string, key: string): Record<string, string | undefined> =>
  Object.fromEntries(Object.keys(HELPER_LIMITS).map((l) => [l, catalog[l]?.[ns]?.[key]?.defaultMessage]));

/** The budget findings of the catalog (of the ids of `only`, when given) and of the baseline itself, all errors. */
export function budgetFindings(catalog: Catalog, baseline: Baseline, only?: Set<string>): string[] {
  const out: string[] = [];
  for (const [ns, entries] of Object.entries(catalog[SOURCE] ?? {})) {
    for (const key of Object.keys(entries)) {
      const id = `${ns}:${key}`;
      if (only && !only.has(id)) continue;
      out.push(...helperFindings(ns, key, textsOf(catalog, ns, key), baseline[id]));
    }
  }
  for (const [id, reason] of staleEntries(catalog, baseline)) {
    out.push(`${rel(baselinePath)} ${id}: ${reason}; the list only shrinks: run scripts/i18n.sh prune-baseline`);
  }
  return out;
}

/** The baseline entries that must go: id -> why. */
function staleEntries(catalog: Catalog, baseline: Baseline): Map<string, string> {
  const out = new Map<string, string>();
  for (const id of Object.keys(baseline)) {
    const [ns, key] = id.split(/:(.*)/s);
    if (!catalog[SOURCE]?.[ns]?.[key]) out.set(id, "the key no longer exists");
    else if (!isHelperText(ns, key)) out.set(id, "the key is not a helper text");
    else if (Object.entries(HELPER_LIMITS).every(([l, { max }]) => lengthIn(l, textsOf(catalog, ns, key)[l]) <= max))
      out.set(id, "the text is now within the limit");
  }
  return out;
}

/**
 * Removes the entries that must go and lowers the recorded lengths to the present ones; never adds an entry or
 * raises a length. Returns the ids it removed.
 */
export function pruneBaseline(catalog: Catalog, baseline: Baseline = loadBaseline()): string[] {
  const stale = staleEntries(catalog, baseline);
  const next: Baseline = {};
  for (const id of Object.keys(baseline).sort()) {
    if (stale.has(id)) continue;
    const [ns, key] = id.split(/:(.*)/s);
    const texts = textsOf(catalog, ns, key);
    next[id] = Object.fromEntries(
      Object.keys(HELPER_LIMITS).map((l) => [l, Math.min(baseline[id][l] ?? Infinity, lengthIn(l, texts[l]))]),
    );
  }
  if (existsSync(baselinePath) || Object.keys(next).length) writeFileAtomic(baselinePath, toJSON(next));
  return [...stale.keys()];
}
