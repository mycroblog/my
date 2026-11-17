// SPDX-License-Identifier: AGPL-3.0-only

// Copies the license notices of the third-party data and files the client ships but that are not npm dependencies (and
// so are outside `web:licenses`) — the Unicode emoji data of the picker (@my/app-features'
// emoji/unicode/LICENSE-Unicode.txt, ADR-0091 §1) and the OFL.txt of the Noto Color Emoji fonts of @my/design-system
// (ADR-0091 §13) — into the build's licenses/ under their own names, next to the notices of the npm packages: Go serves
// them at /licenses/.
import { readFileSync } from "node:fs";
import { basename } from "node:path";
import type { Plugin } from "vite";

export function bundledLicenses(files: readonly string[]): Plugin {
  return {
    name: "my:licenses",
    apply: "build",
    generateBundle() {
      for (const file of files) {
        this.emitFile({ type: "asset", fileName: `licenses/${basename(file)}`, source: readFileSync(file) });
      }
    },
  };
}
