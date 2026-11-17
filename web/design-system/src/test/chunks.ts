// SPDX-License-Identifier: AGPL-3.0-only

// The chunks the design system loads on first use (the emoji picker, the plain-text editor), loaded with the module
// that imports this one: a component test's support imports it, so that a test which opens one waits for what the
// part does, never for the machine to read and compile a chunk. That a chunk loads on first use, and what shows while
// it does, is the E2E tests' and each part's own test of its loading.

import "../editor";
import "../emoji-picker";
