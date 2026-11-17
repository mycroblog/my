// SPDX-License-Identifier: AGPL-3.0-only

// What a change of screen does with no browser at all, and the rules its View Transitions run by. What it does in a
// document is in motion.dom.test.tsx.

import { expect, test } from "bun:test";
import { routerTransitions, runTransition, type TransitionKind } from "./motion";
import { transitionRules } from "./motion-css";

test("runTransition changes the screen at once outside a browser", async () => {
  let updated = false;
  await runTransition(
    "push",
    () => {
      updated = true;
    },
    false,
  );
  expect(updated).toBe(true);
});

test("a router runs no View Transition where the browser has none", () => {
  const kindOf = (to: string): TransitionKind => (to === "/" ? "replace" : "push");
  expect(routerTransitions(kindOf, false)).toBe(false);
});

test("push and pop mirror in a right-to-left page, and reduced motion stops every View Transition", () => {
  expect(transitionRules[':root[dir="rtl"]']).toEqual({ "--my-direction": "-1" });
  expect(JSON.stringify(transitionRules["@keyframes my-push-in"])).toContain("var(--my-direction)");
  expect(transitionRules["@media (prefers-reduced-motion: reduce)"]).toEqual({
    "::view-transition-group(*), ::view-transition-old(*), ::view-transition-new(*)": { animation: "none !important" },
  });
});
