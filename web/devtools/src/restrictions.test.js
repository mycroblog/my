// SPDX-License-Identifier: AGPL-3.0-only
// Proves the rules of restrictions.js as the workspace runs them: `bun --bun oxlint` with .oxlintrc.json lints the
// fixtures (web/devtools/fixtures/restrictions/ and the fixtures/ of the layers), and each finding must be one that a
// fixture line expects with an `error: <Component> prop` comment (design-system-props), an `error: layers @my/<x>`
// comment (layer-imports), an `error: platform <what>` comment (platform-globals), an `error: e2e <what>` comment
// (e2e-queries) or an `error: e2e-flaky <what>` comment (e2e-flaky), and each expected one must be found. Oxlint's RuleTester refuses to run on Bun (ADR-0025
// §8, §10). .oxlintrc.json ignores the fixtures, so that no other lint run fails on them; this lints copies of them
// beside them, under the same overrides, and removes the copies.

import { expect, test } from "bun:test";
import { cpSync, readdirSync, rmSync } from "node:fs";

const root = new URL("../../../", import.meta.url).pathname;
/** The packages with fixtures, each in the directory whose rules it proves. */
const dirs = [
  "web/devtools",
  "web/design-system",
  "web/app-components",
  "web/app-features",
  "web/app",
  "web/platform",
  "web/e2e",
];
const fixtures = dirs.flatMap((dir) =>
  readdirSync(`${root}${dir}/fixtures`, { recursive: true, encoding: "utf8" })
    .filter((name) => /\.tsx?$/.test(name))
    .map((name) => `${dir}/fixtures/${name}`),
);

/** What each rule's message says of a finding, as a fixture writes it after `error: `. */
const findings = {
  "restrict(design-system-props)": (/** @type {string} */ message) => {
    const [, component, prop] = /^(<\S+>) of @my\/design-system gets `(\w+)`/.exec(message) ?? [];
    return `${component} ${prop}`;
  },
  "restrict(layer-imports)": (/** @type {string} */ message) =>
    `layers ${/^@my\/\S+ imports (@my\/[\w-]+)/.exec(message)?.[1]}`,
  "restrict(platform-globals)": (/** @type {string} */ message) => `platform ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(e2e-queries)": (/** @type {string} */ message) => `e2e ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(e2e-flaky)": (/** @type {string} */ message) => `e2e-flaky ${/^`([^`]+)`/.exec(message)?.[1]}`,
};

/** The findings of the rules, as `file:line <what>`. */
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
    .filter((d) => Object.hasOwn(findings, d.code))
    .map((d) => {
      const what = findings[/** @type {keyof typeof findings} */ (d.code)](d.message);
      return `${d.filename.replace(`/${copy}/`, "/fixtures/")}:${d.labels[0]?.span.line} ${what}`;
    })
    .sort();
}

/** What the fixtures expect, as `file:line <what>`. */
async function expected() {
  const found = [];
  for (const file of fixtures) {
    const lines = (await Bun.file(`${root}${file}`).text()).split("\n");
    for (const [index, line] of lines.entries()) {
      const match =
        /error: (<\S+> (?:className|style)|layers @my\/[\w-]+|platform (?:"\w+" in|\S+)|e2e \S+|e2e-flaky .*\S)/.exec(
          line,
        );
      if (match) found.push(`${file}:${index + 1} ${match[1]}`);
    }
  }
  return found.sort();
}

test("the rules find exactly what the fixtures expect, and nothing in the allowed lines", async () => {
  expect(fixtures).toContain("web/devtools/fixtures/restrictions/allowed.tsx");
  expect(fixtures).toContain("web/design-system/fixtures/inside.tsx");
  expect(fixtures).toContain("web/app/fixtures/layers.tsx");
  expect(fixtures).toContain("web/platform/fixtures/inside.ts");
  expect(fixtures).toContain("web/e2e/fixtures/queries.ts");
  expect(fixtures).toContain("web/e2e/fixtures/flaky.ts");
  expect(fixtures).toContain("web/e2e/fixtures/specs/timing.ts");
  expect(fixtures).toContain("web/e2e/fixtures/config/playwright.config.ts");
  const want = await expected();
  expect(want.some((line) => line.includes(" e2e-flaky "))).toBe(true);
  expect(want.some((line) => line.endsWith(" className") || line.endsWith(" style"))).toBe(true);
  expect(want.some((line) => line.includes(" layers @my/"))).toBe(true);
  expect(want.some((line) => line.includes(" platform "))).toBe(true);
  expect(want.some((line) => line.includes(" e2e "))).toBe(true);
  expect(lint()).toEqual(want);
}, 60_000);
