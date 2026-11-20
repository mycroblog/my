// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { readdirSync } from "node:fs";
import { screenshotsDir, shotFiles } from "./list";

test("every image of the documentation site's screenshots is a shot of scripts/task.sh web:docs-screenshots", () => {
  expect(readdirSync(screenshotsDir).toSorted()).toEqual(shotFiles().toSorted());
});
