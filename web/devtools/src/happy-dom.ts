// SPDX-License-Identifier: AGPL-3.0-only

// The preload of the component tests of a package of web/ (`*.dom.test.tsx`, ADR-0171 §4): happy-dom is the global
// DOM before a test module loads (`bun test --preload`, tasks/web.yml).

import { GlobalRegistrator } from "@happy-dom/global-registrator";

GlobalRegistrator.register({ url: "http://localhost/" });

await import("./component-tests");
