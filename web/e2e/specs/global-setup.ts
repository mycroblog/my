// SPDX-License-Identifier: AGPL-3.0-only

// Starts the demo server with the scenario (src/server/demo.ts) in .tmp/e2e/server for the run, and stops it after.
// The specs read its origin, the seed's state and the settings of `bin/my` against its database from E2E_DEMO_URL,
// E2E_DEMO_STATE and E2E_DEMO_ENV (specs/test.ts), and the fake push service's origin and CA from E2E_PUSH_ORIGIN and
// E2E_PUSH_CA (src/web-push.ts).

import { join } from "node:path";
import { startDemo } from "../src/server/demo";
import { root } from "../src/server/paths";

export default async function globalSetup() {
  const demo = await startDemo({ dir: join(root, ".tmp/e2e/server"), output: "file" });
  process.env["E2E_DEMO_URL"] = demo.url;
  process.env["E2E_DEMO_STATE"] = demo.stateFile;
  process.env["E2E_DEMO_ENV"] = demo.envFile;
  process.env["E2E_PUSH_ORIGIN"] = demo.push.origin;
  process.env["E2E_PUSH_CA"] = demo.push.ca;
  return demo.stop;
}
