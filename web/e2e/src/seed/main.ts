// SPDX-License-Identifier: AGPL-3.0-only

// The seed as a command, which the demo server (server/demo.ts) runs once `my serve` answers:
//
//   bun --conditions=@mycroblog/source web/e2e/src/seed/main.ts --server <origin> --state <file>
//
// It runs `bin/my` (E2E_MY_BIN overrides it) in this process's environment, which names the server's database, reads
// the state of an earlier run from the file when it exists, and writes the state back.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { parseArgs } from "node:util";
import { myBin } from "../server/paths";
import { seed, type SeedState } from "./seed";

const { values } = parseArgs({ options: { server: { type: "string" }, state: { type: "string" } } });
if (values.server === undefined || values.state === undefined)
  throw new Error("seed: --server <origin> and --state <file> are required");

const state = existsSync(values.state) ? (JSON.parse(readFileSync(values.state, "utf8")) as SeedState) : undefined;
const next = await seed({
  server: values.server,
  runMy: (args, input = "") => execFileSync(myBin, args, { input, encoding: "utf8", stdio: ["pipe", "pipe", "pipe"] }),
  ...(state === undefined ? {} : { state }),
  log: (line) => {
    console.log(`seed: ${line}`);
  },
});
writeFileSync(values.state, JSON.stringify(next, null, 2), { mode: 0o600 });
console.log("seed: done");
