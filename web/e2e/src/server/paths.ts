// SPDX-License-Identifier: AGPL-3.0-only

// Where the demo server finds the repository and the binary.

import { join, resolve } from "node:path";

/** The root of the repository. */
export const root = resolve(import.meta.dirname, "../../../..");

/** The binary that `scripts/task.sh go:bin GO_TAGS=e2e_fakes` writes; E2E_MY_BIN overrides it. */
export const myBin = process.env["E2E_MY_BIN"] ?? join(root, "bin/my");
