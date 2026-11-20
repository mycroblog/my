// SPDX-License-Identifier: AGPL-3.0-only

// The demo server of the E2E tests with the scenario (specs/global-setup.ts), on the fixed port `shotsPort`, since the
// screens show the server's host and port in each handle ("@yuki@localhost:8789"); something that listens there
// already fails the run. It sets what the specs' fixtures read (specs/test.ts), and stops the server after.

import { join } from "node:path";
import { startDemo } from "../src/server/demo";
import { root } from "../src/server/paths";

const shotsPort = 8789;

export default async function globalSetup() {
  const demo = await startDemo({ dir: join(root, ".tmp/shots/server"), port: shotsPort, output: "file" });
  process.env["E2E_DEMO_URL"] = demo.url;
  process.env["E2E_DEMO_STATE"] = demo.stateFile;
  process.env["E2E_DEMO_ENV"] = demo.envFile;
  process.env["E2E_DEMO_LOG"] = demo.logFile ?? "";
  process.env["E2E_PUSH_ORIGIN"] = demo.push.origin;
  process.env["E2E_PUSH_CA"] = demo.push.ca;
  process.env["E2E_OIDC_ISSUER"] = demo.oidc.issuer;
  return demo.stop;
}
