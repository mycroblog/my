// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { supportsViewTransitions } from "./transitions";

const globals = globalThis as Record<string, unknown>;

afterEach(() => {
  delete globals.document;
  delete globals.CSS;
});

test("a browser runs View Transitions with types only when it has both", () => {
  expect(supportsViewTransitions()).toBe(false);
  globals.document = { startViewTransition: () => undefined };
  globals.CSS = { supports: (q: string) => q === "selector(:active-view-transition-type(a))" };
  expect(supportsViewTransitions()).toBe(true);
  globals.CSS = { supports: () => false };
  expect(supportsViewTransitions()).toBe(false);
  globals.document = {};
  globals.CSS = { supports: () => true };
  expect(supportsViewTransitions()).toBe(false);
});
