// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { goFile, renderGo, themeTokens } from "./theme-tokens";

test("the committed Go token list is up to date (scripts/task.sh web:tokens-gen rewrites it)", async () => {
  expect(await Bun.file(goFile).text()).toBe(renderGo());
});

test("the avatar's backing and the components' sizes are fixed", () => {
  const names = themeTokens().map((t) => t.name);
  expect(names).toContain("color-surface");
  expect(names).toContain("text-body");
  expect(names).not.toContain("color-avatar-backing");
  expect(names).not.toContain("control-timeline-height");
  expect(names).not.toContain("shadow-overlay");
});
