// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { readdirSync } from "node:fs";
import { checkFrame } from "./check";

const fixtures = new URL("../fixtures/", import.meta.url);
const read = (name: string) => Bun.file(new URL(name, fixtures)).text();

test("a layout that keeps the rules has no problem", async () => {
  expect(checkFrame(await read("keeps.html"))).toEqual([]);
});

// Each fixture but keeps.html breaks the rule its name starts with.
const rules = ["external", "inline-styles", "lang-dir", "tables"];
const broken = readdirSync(fixtures).filter((f) => f.endsWith(".html") && f !== "keeps.html");
const ruleOf = (name: string) => rules.find((r) => name.startsWith(`${r}-`) || name === `${r}.html`);

test("every fixture names a rule, and every rule has one", () => {
  expect(broken.map(ruleOf)).not.toContain(undefined);
  expect([...new Set(broken.map(ruleOf))].sort()).toEqual(rules);
});

test.each(broken)("%s is refused for its rule", async (name) => {
  const problems = checkFrame(await read(name));
  expect(problems.some((p) => p.startsWith(`${ruleOf(name)}: `))).toBe(true);
});
