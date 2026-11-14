// SPDX-License-Identifier: AGPL-3.0-only

// The decider's demo (`scripts/task.sh web:demo`, ADR-0015 §8): the demo server (demo.ts) in .tmp/demo/ on DEMO_PORT
// (8787 by default), with its output here, until Ctrl-C. It prints the URL and the people's sign-ins once the scenario
// is in.

import { join } from "node:path";
import { people } from "../scenario";
import { loginOf } from "../seed/seed";
import { startDemo } from "./demo";
import { root } from "./paths";

const demo = await startDemo({
  dir: join(root, ".tmp/demo"),
  port: Number(process.env["DEMO_PORT"] ?? "8787"),
  output: "inherit",
});
const totp = Object.keys(demo.state.totp);
console.log(
  [
    "",
    `demo: ready at ${demo.url}`,
    "Sign in as (username, email, password):",
    ...people.map((p) => `  ${loginOf(p)}`),
    ...(totp.length === 0
      ? []
      : [`Two-factor secrets (add them to an authenticator app) are in ${demo.stateFile}: ${totp.join(", ")}`]),
    "Ctrl-C stops the server and removes its data.",
  ].join("\n"),
);

const stop = () => {
  void demo.stop().then(() => process.exit(0));
};
process.on("SIGINT", stop);
process.on("SIGTERM", stop);
