// SPDX-License-Identifier: AGPL-3.0-only

// The preload of the component tests of a package of web/ (`*.dom.test.tsx`, ADR-0171 §4): happy-dom is the global
// DOM before a test module loads (`bun test --preload`, which dom-tests.ts runs for the package's `test` script).

import { GlobalRegistrator } from "@happy-dom/global-registrator";
import { withoutReactTracks } from "./react-tracks";

GlobalRegistrator.register({ url: "http://localhost/" });

await withoutReactTracks(() => import("./component-tests"));
