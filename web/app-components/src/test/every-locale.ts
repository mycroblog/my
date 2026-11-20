// SPDX-License-Identifier: AGPL-3.0-only

// The preload of the package's tests: the strings of every locale are read before a test module loads, so that a test
// renders in any locale at once, as a page does once it loaded the reader's (strings-every.ts). The package's `test`
// script preloads it, and so does the preload of the tests on a DOM (@my/devtools, component-tests.ts).

import { readEveryLocale } from "../strings-every";

readEveryLocale();
