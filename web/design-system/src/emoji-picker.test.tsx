// SPDX-License-Identifier: AGPL-3.0-only

// What keeps the emoji picker fast with thousands of emoji (ADR-0091 §14, §15): the grid draws only the rows in view
// and a few beyond, however many emoji it holds, while it tells the size of the whole; and the picker's module stays
// out of the design system's entry, which only names its types and where it opens.

import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { renderToStaticMarkup } from "react-dom/server";
import { EmojiPicker, type EmojiSection, type PickerEmoji } from "./emoji-picker";

/** `n` emoji of a section, each its own. */
const many = (n: number, prefix: string): PickerEmoji[] =>
  Array.from({ length: n }, (_, i) => ({ key: `${prefix}${String(i)}`, name: `emoji ${String(i)}`, text: "😀" }));

const sections: EmojiSection[] = [
  { id: "a", label: "A", icon: "emojiSmileys", emojis: many(2000, "a") },
  { id: "b", label: "B", icon: "emojiPeople", emojis: many(2000, "b") },
];

test("the grid draws a bounded number of cells however many emoji it holds, and tells the size of the whole", () => {
  const html = renderToStaticMarkup(
    <EmojiPicker sections={sections} search={() => []} onSelect={() => undefined} labels={{ tone: "", tones: [] }} />,
  );
  const cells = html.match(/role="gridcell"/g)?.length ?? 0;
  // A popover's most height of rows in view, and four rows on each side, of eight columns where nothing is laid out.
  expect(cells).toBeGreaterThan(0);
  expect(cells).toBeLessThanOrEqual(8 * 16);
  // Two headings and 4000 emoji in rows of eight.
  expect(html).toContain(`aria-rowcount="${String(2 + 4000 / 8)}"`);
});

test("the design system's entry names the picker's types alone, so that its module is a chunk of its own", () => {
  const entry = readFileSync(new URL("./index.ts", import.meta.url), "utf8");
  const imports = entry.split("\n").filter((line) => line.includes('from "./emoji-picker"'));
  expect(imports.length).toBeGreaterThan(0);
  for (const line of imports) expect(line).toMatch(/^export type /);
});
