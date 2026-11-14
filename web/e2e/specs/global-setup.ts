// SPDX-License-Identifier: AGPL-3.0-only

// Starts the demo server with the scenario (src/server/demo.ts) in .tmp/e2e/server for the run, and stops it after.
// The specs read its origin and the seed's state from E2E_DEMO_URL and E2E_DEMO_STATE (specs/test.ts).

import { join } from "node:path";
import { startDemo } from "../src/server/demo";
import { root } from "../src/server/paths";

export default async function globalSetup() {
  const demo = await startDemo({ dir: join(root, ".tmp/e2e/server"), output: "file" });
  process.env["E2E_DEMO_URL"] = demo.url;
  process.env["E2E_DEMO_STATE"] = demo.stateFile;
  return demo.stop;
}
