// SPDX-License-Identifier: AGPL-3.0-only
// A module of a package's `test/` directory, which the tests share, may import the fake API of @my/e2e as a test may.

import { createFakeApi } from "@my/e2e/fake";
import { scenario } from "@my/e2e"; // error: layers @my/e2e

export const used = [createFakeApi, scenario];
