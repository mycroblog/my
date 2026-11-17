// SPDX-License-Identifier: AGPL-3.0-only
// A test, and a module of a package's `test/` directory (../test/feature.tsx), may also import the fake API of @my/e2e,
// and no other part of it, nothing a story alone may import, and no layer above its own. It is in a hidden directory,
// which the test runner does not search for tests.

import { useHomeTimeline } from "@my/app-data";
import { createFakeApi } from "@my/e2e/fake";
import { scenario } from "@my/e2e"; // error: layers @my/e2e
import { Options } from "@my/devtools/options"; // error: layers @my/devtools
import { AppRoot } from "@my/app"; // error: layers @my/app

export const used = [useHomeTimeline, createFakeApi, scenario, Options, AppRoot];
