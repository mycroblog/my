// SPDX-License-Identifier: AGPL-3.0-only

// What keeps the emoji picker out of the pages that never open it (ADR-0091 §15): the picker's module stays out of the
// design system's entry, which only names its types and where it opens.

import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";

test("the design system's entry names the picker's types alone, so that its module is a chunk of its own", () => {
  const entry = readFileSync(new URL("./index.ts", import.meta.url), "utf8");
  const imports = entry.split("\n").filter((line) => line.includes('from "./emoji-picker"'));
  expect(imports.length).toBeGreaterThan(0);
  for (const line of imports) expect(line).toMatch(/^export type /);
});
