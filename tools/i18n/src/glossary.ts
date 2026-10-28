// SPDX-License-Identifier: AGPL-3.0-only
// The glossary of the catalogs (tools/i18n/glossary.json, web-client/16-look-states-and-wording.md G21): per locale,
// the preferred term of a thing and the variants check.ts rejects in the text of a message.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { repoRoot } from "./catalog";

/** One thing: the term to use and the variants (regular expressions over the message text) to avoid. */
export type Term = { use: string; avoid: string[]; note?: string };
/** locale -> terms. A locale matches its own entries only; pseudo-locales are generated and never checked. */
export type Glossary = Record<string, Term[]>;

export const glossaryPath = join(repoRoot, "tools", "i18n", "glossary.json");

export const loadGlossary = (path: string = glossaryPath): Glossary => JSON.parse(readFileSync(path, "utf8"));

/** The findings for the text of one message of `locale`: one per banned variant it contains. */
export function glossaryFindings(glossary: Glossary, locale: string, text: string): string[] {
  const out: string[] = [];
  for (const term of glossary[locale] ?? []) {
    for (const avoid of term.avoid) {
      const found = new RegExp(avoid, "mu").exec(text);
      if (found) out.push(`"${found[0]}" is not in the glossary; use "${term.use}" (tools/i18n/glossary.json)`);
    }
  }
  return out;
}
