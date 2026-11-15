// SPDX-License-Identifier: AGPL-3.0-only

// What a version of a post changed in its text from the version before it, word by word: the plain text of the HTML
// the server rendered (its paragraphs and line breaks kept), cut into words by the platform's segmenter in the post's
// language (which cuts Japanese, written without spaces, into its words too), and the longest run they share kept,
// the rest added or removed.

import type { DiffPart } from "@my/design-system";

const entities: Record<string, string> = { amp: "&", lt: "<", gt: ">", quot: '"', apos: "'", nbsp: " " };

/** The text of the server's HTML: a paragraph a blank line apart, a line break a line, no tags, entities decoded. */
export function plainText(html: string): string {
  return html
    .replace(/<\/p>\s*<p[^>]*>/g, "\n\n")
    .replace(/<br\s*\/?>/g, "\n")
    .replace(/<[^>]+>/g, "")
    .replace(/&(#x?[0-9a-f]+|[a-z]+);/gi, (whole, name: string) => {
      if (name.startsWith("#x") || name.startsWith("#X")) return String.fromCodePoint(parseInt(name.slice(2), 16));
      if (name.startsWith("#")) return String.fromCodePoint(Number(name.slice(1)));
      return entities[name.toLowerCase()] ?? whole;
    })
    .trim();
}

/** The words of `text` and what lies between them, in order. */
function words(text: string, lang: string): string[] {
  return Array.from(new Intl.Segmenter(lang, { granularity: "word" }).segment(text), (s) => s.segment);
}

/** The runs of `after`'s text against `before`'s: the same, added, removed, adjacent runs of a kind joined. */
export function diffWords(before: string, after: string, lang: string): DiffPart[] {
  const a = words(before, lang);
  const b = words(after, lang);
  // The length of the longest common run of a[i..] and b[j..], for every i and j, in one row-major table.
  const width = b.length + 1;
  const table = new Uint32Array((a.length + 1) * width);
  const lcs = (i: number, j: number) => table[i * width + j] ?? 0;
  for (let i = a.length - 1; i >= 0; i--)
    for (let j = b.length - 1; j >= 0; j--)
      table[i * width + j] = a[i] === b[j] ? lcs(i + 1, j + 1) + 1 : Math.max(lcs(i + 1, j), lcs(i, j + 1));
  const ops: DiffPart[] = [];
  let i = 0;
  let j = 0;
  for (;;) {
    const x = a[i];
    const y = b[j];
    if (x === undefined && y === undefined) break;
    if (x !== undefined && x === y) {
      ops.push({ text: x, kind: "same" });
      i++;
      j++;
    } else if (x !== undefined && (y === undefined || lcs(i + 1, j) >= lcs(i, j + 1))) {
      ops.push({ text: x, kind: "removed" });
      i++;
    } else if (y !== undefined) {
      ops.push({ text: y, kind: "added" });
      j++;
    }
  }
  return hunks(ops);
}

const blank = /^\s*$/;

/**
 * The words in order as runs a reader takes in: a change and the next one with only spaces between them are one
 * change, whose removed words read whole before the words that took their place, and a change starts and ends at a
 * word, its spaces left unmarked.
 */
function hunks(ops: readonly DiffPart[]): DiffPart[] {
  const parts: DiffPart[] = [];
  const push = (text: string, kind: DiffPart["kind"]) => {
    if (text === "") return;
    const last = parts.at(-1);
    if (last?.kind === kind) last.text += text;
    else parts.push({ text, kind });
  };
  let k = 0;
  while (k < ops.length) {
    const at = ops[k];
    if (at === undefined) break;
    if (at.kind === "same") {
      push(at.text, "same");
      k++;
      continue;
    }
    // A change: up to the last changed word before a word both versions share.
    let end = k;
    for (const [n, o] of ops.entries()) {
      if (n < k) continue;
      if (o.kind !== "same") end = n;
      else if (!blank.test(o.text)) break;
    }
    let removed = "";
    let added = "";
    for (const o of ops.slice(k, end + 1)) {
      if (o.kind !== "added") removed += o.text;
      if (o.kind !== "removed") added += o.text;
    }
    const kept = added === "" ? removed : added;
    push(/^\s*/.exec(kept)?.[0] ?? "", "same");
    push(removed.trim(), "removed");
    push(added.trim(), "added");
    push(/\s*$/.exec(kept)?.[0] ?? "", "same");
    k = end + 1;
  }
  return parts;
}
