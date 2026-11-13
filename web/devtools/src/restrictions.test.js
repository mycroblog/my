// SPDX-License-Identifier: AGPL-3.0-only
// Proves the rules of restrictions.js as the workspace runs them: `bun --bun oxlint` with .oxlintrc.json lints the
// fixtures (web/devtools/fixtures/restrictions/, web/design-system/fixtures/), and each finding must be one that a
// fixture line expects with an `error: <Component> prop` comment, and each expected one must be found. Oxlint's
// RuleTester refuses to run on Bun (ADR-0025 §8, §10). .oxlintrc.json ignores the fixtures, so that no other lint run
// fails on them; this lints copies of them beside them, under the same overrides, and removes the copies.

import { expect, test } from "bun:test";
import { cpSync, readdirSync, rmSync } from "node:fs";

const root = new URL("../../../", import.meta.url).pathname;
const dirs = ["web/devtools", "web/design-system"];
const fixtures = [
  ...readdirSync(`${root}web/devtools/fixtures/restrictions`).map(
    (name) => `web/devtools/fixtures/restrictions/${name}`,
  ),
  ...readdirSync(`${root}web/design-system/fixtures`).map((name) => `web/design-system/fixtures/${name}`),
];

/** The findings of restrict/design-system-props, as `file:line <Component> prop`. */
function lint() {
  const copy = `.fixtures-${process.pid}-${Date.now()}`;
  let output = "";
  try {
    for (const dir of dirs) cpSync(`${root}${dir}/fixtures`, `${root}${dir}/${copy}`, { recursive: true });
    const copies = fixtures.map((file) => file.replace("/fixtures/", `/${copy}/`));
    const run = Bun.spawnSync([process.execPath, "--bun", "oxlint", "--format", "json", ...copies], { cwd: root });
    output = run.stdout.toString();
  } finally {
    for (const dir of dirs) rmSync(`${root}${dir}/${copy}`, { recursive: true, force: true });
  }
  /** @type {{ diagnostics: { code: string, message: string, filename: string, labels: { span: { line: number } }[] }[] }} */
  const report = JSON.parse(output.slice(output.indexOf("{")));
  return report.diagnostics
    .filter((d) => d.code === "restrict(design-system-props)")
    .map((d) => {
      const [, component, prop] = /^(<\S+>) of @my\/design-system gets `(\w+)`/.exec(d.message) ?? [];
      return `${d.filename.replace(`/${copy}/`, "/fixtures/")}:${d.labels[0]?.span.line} ${component} ${prop}`;
    })
    .sort();
}

/** What the fixtures expect, as `file:line <Component> prop`. */
async function expected() {
  const found = [];
  for (const file of fixtures) {
    const lines = (await Bun.file(`${root}${file}`).text()).split("\n");
    for (const [index, line] of lines.entries()) {
      const match = /error: (<\S+>) (className|style)/.exec(line);
      if (match) found.push(`${file}:${index + 1} ${match[1]} ${match[2]}`);
    }
  }
  return found.sort();
}

test("design-system-props finds exactly what the fixtures expect, and nothing in the allowed ones", async () => {
  expect(fixtures).toContain("web/devtools/fixtures/restrictions/allowed.tsx");
  expect(fixtures).toContain("web/design-system/fixtures/inside.tsx");
  const want = await expected();
  expect(want.length).toBeGreaterThan(0);
  expect(lint()).toEqual(want);
}, 60_000);
