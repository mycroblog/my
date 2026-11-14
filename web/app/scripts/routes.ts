// SPDX-License-Identifier: AGPL-3.0-only
// Generates src/routeTree.gen.ts from the file-based routes of src/routes without Vite (ADR-0081 §1), so that tsc,
// Oxlint, knip and the tests have the route tree before a build: `scripts/task.sh web:routes`. The options are those of
// the tanstackRouter plugin in vite.config.ts; keep them the same, or the build writes another tree.
import { fileURLToPath } from "node:url";
import { Generator, getConfig } from "@tanstack/router-generator";

const root = fileURLToPath(new URL("..", import.meta.url));
const config = getConfig({ target: "react", autoCodeSplitting: true }, root);
await new Generator({ config, root }).run();
