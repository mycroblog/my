// SPDX-License-Identifier: AGPL-3.0-only
import { afterAll, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const script = join(import.meta.dir, "licenses.ts");
const dirs: string[] = [];
afterAll(() => {
  for (const d of dirs) rmSync(d, { recursive: true, force: true });
});

/** A node_modules directory with one package of each given name and license. */
function nodeModules(packages: Record<string, string>): string {
  const root = mkdtempSync(join(tmpdir(), "licenses-test-"));
  dirs.push(root);
  for (const [name, license] of Object.entries(packages)) {
    const dir = join(root, name);
    mkdirSync(dir, { recursive: true });
    writeFileSync(join(dir, "package.json"), JSON.stringify({ name, version: "1.0.0", license }));
  }
  return root;
}

function run(...args: string[]) {
  const p = Bun.spawnSync(["bun", "run", script, ...args], { stderr: "pipe", stdout: "pipe" });
  return { code: p.exitCode, stderr: p.stderr.toString() };
}

const exempt = "@img/sharp-libvips-*=LGPL-3.0-or-later";

test("--exempt admits the named packages with that license", () => {
  const dir = nodeModules({ "@img/sharp-libvips-linux-x64": "LGPL-3.0-or-later", "left-pad": "MIT" });
  expect(run("MIT", "--exempt", exempt, dir).code).toBe(0);
});

test("without --exempt the same packages fail", () => {
  const dir = nodeModules({ "@img/sharp-libvips-linux-x64": "LGPL-3.0-or-later" });
  const r = run("MIT", dir);
  expect(r.code).toBe(1);
  expect(r.stderr).toContain("@img/sharp-libvips-linux-x64@1.0.0: LGPL-3.0-or-later");
});

test("--exempt admits no other package with that license, nor another license of the named packages", () => {
  const dir = nodeModules({ "some-lgpl-lib": "LGPL-3.0-or-later", "@img/sharp-libvips-darwin-arm64": "GPL-3.0-only" });
  const r = run("MIT", "--exempt", exempt, dir);
  expect(r.code).toBe(1);
  expect(r.stderr).toContain("some-lgpl-lib@1.0.0: LGPL-3.0-or-later");
  expect(r.stderr).toContain("@img/sharp-libvips-darwin-arm64@1.0.0: GPL-3.0-only");
});

test("a malformed --exempt entry is a usage error", () => {
  expect(run("MIT", "--exempt", "no-license", nodeModules({})).code).toBe(2);
});
