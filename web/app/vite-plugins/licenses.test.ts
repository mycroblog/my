// SPDX-License-Identifier: AGPL-3.0-only
// Builds a small module with the licenses plugin as `vite build` would, and proves that each license file it is given
// lands in the build's licenses/ under its own name with its bytes unchanged (ADR-0091 §13). That the app's build
// carries the notice of each font it ships is scripts/build-check.ts, and that the server serves the files of licenses/
// is TestStaticFiles of internal/interface/web.

import { afterAll, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { build, type Rolldown } from "vite";
import { bundledLicenses } from "./licenses";

const root = mkdtempSync(join(tmpdir(), "licenses-test-"));
afterAll(() => {
  rmSync(root, { recursive: true, force: true });
});

/** The assets of a build of one module with the plugin given `files`, by their file names. */
async function assets(files: readonly string[]): Promise<Map<string, string | Uint8Array>> {
  const entry = join(root, "entry.js");
  writeFileSync(entry, "export const answer = 42;\n");
  const result = await build({
    root,
    configFile: false,
    logLevel: "silent",
    plugins: [bundledLicenses(files)],
    build: { write: false, lib: { entry, formats: ["es"], fileName: "entry" } },
  });
  const outputs = (Array.isArray(result) ? result : [result]) as Rolldown.RolldownOutput[];
  return new Map(
    outputs.flatMap((o) => o.output).flatMap((c) => (c.type === "asset" ? [[c.fileName, c.source] as const] : [])),
  );
}

const text = (source: string | Uint8Array | undefined) =>
  source === undefined ? undefined : typeof source === "string" ? source : new TextDecoder().decode(source);

test("each license file lands in licenses/ under its own name, unchanged", async () => {
  const ofl = join(root, "OFL.txt");
  const other = join(root, "LICENSE-unicode.txt");
  writeFileSync(ofl, "SIL OPEN FONT LICENSE Version 1.1\nCopyright 2013 Google LLC\n");
  writeFileSync(other, "UNICODE LICENSE V3\n");
  const out = await assets([ofl, other]);
  expect(text(out.get("licenses/OFL.txt"))).toBe(readFileSync(ofl, "utf8"));
  expect(text(out.get("licenses/LICENSE-unicode.txt"))).toBe(readFileSync(other, "utf8"));
});
