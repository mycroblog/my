// SPDX-License-Identifier: AGPL-3.0-only

// The chunks the features load on first use (the composer's view and its editor, the emoji and their names, and the
// design system's own), loaded with the module that imports this one: a component test's support imports it, so that
// a test which opens the composer or the emoji picker waits for what the feature does, never for the machine to read
// and compile a chunk. That a chunk loads on first use, and what shows while it does, is the E2E tests' and the
// composer's own tests of its loading.

import "@my/design-system/test/chunks";
import "../compose/composer";
import "../compose/text";
import "../emoji/unicode/set";
import "../emoji/unicode/base.gen.json";
import "../emoji/unicode/en.gen.json";
import "../emoji/unicode/ja.gen.json";
