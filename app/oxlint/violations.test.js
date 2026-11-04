// SPDX-License-Identifier: AGPL-3.0-only
// Proves that Oxlint, with app/.oxlintrc.json and the JS plugins of the installed versions, reports the
// violation of each 強制 row of docs/architecture/web-client/README.md that lint checks (ADR-0025): it copies the
// fixtures of violations/ into a feature of app/web, where the rules of a feature apply, lints them, and
// requires exactly the reports that each line's `expect:` comment names (as `plugin(rule)`, the codes of
// Oxlint's JSON reporter). Run with `node --test` from app/ (part of `scripts/task.sh web:lint`).
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { copyFileSync, readFileSync, readdirSync, rmSync } from "node:fs";
import path from "node:path";
import { after, it } from "node:test";
import { appDir } from "./config.js";

const fixtures = path.join(import.meta.dirname, "violations");
const target = path.join(appDir, "web/src/features/timeline/components");
const copies = readdirSync(fixtures).map((name) => ({ from: path.join(fixtures, name), to: path.join(target, name) }));

/** @param {string} file @returns {string[]} "line code", sorted */
function expected(file) {
  return readFileSync(file, "utf8")
    .split("\n")
    .flatMap((text, i) => {
      const expect = /expect: (.*)$/.exec(text)?.[1] ?? "";
      return [...expect.matchAll(/[@\w/-]+\([\w/-]+\)/g)].map(([code]) => `${String(i + 1)} ${code}`);
    })
    .sort();
}

for (const { from, to } of copies) copyFileSync(from, to);
after(() => {
  for (const { to } of copies) rmSync(to, { force: true });
});

const result = spawnSync(path.join(appDir, "node_modules/.bin/oxlint"), ["-f", "json", ...copies.map(({ to }) => to)], {
  cwd: appDir,
  encoding: "utf8",
});
/** @type {{ diagnostics: { code: string, filename: string, labels: { span: { line: number } }[] }[] }} */
const report = JSON.parse(result.stdout);

for (const { from, to } of copies) {
  it(`reports each violation of ${path.basename(from)}`, () => {
    const reported = report.diagnostics
      .filter((d) => path.resolve(appDir, d.filename) === to)
      .map((d) => `${String(d.labels[0]?.span.line)} ${d.code}`)
      .sort();
    assert.deepEqual(reported, expected(from));
  });
}
