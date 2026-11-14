// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { routerTransitions, runTransition, Shared, transitionTypes, type TransitionKind } from "./motion";
import { transitionRules } from "./motion-css";

const globals = globalThis as Record<string, unknown>;

/** A browser with View Transitions and their types, and the reader's motion preference; the calls it saw. */
function browser({ types = true, reduce = false }: { types?: boolean; reduce?: boolean } = {}) {
  const started: { types: string[] }[] = [];
  globals.document = {
    startViewTransition: ({ update, types }: { update: () => void; types: string[] }) => {
      started.push({ types });
      update();
      return { updateCallbackDone: Promise.resolve() };
    },
  };
  globals.CSS = { supports: () => types };
  globals.matchMedia = () => ({ matches: reduce });
  return started;
}

afterEach(() => {
  delete globals.document;
  delete globals.CSS;
  delete globals.matchMedia;
});

test("each kind that moves is the type of its View Transition; none changes the screen at once", () => {
  browser();
  for (const kind of ["push", "pop", "modal", "replace"] as const) expect(transitionTypes(kind)).toEqual([kind]);
  expect(transitionTypes("none")).toBe(false);
});

test("with reduced motion no kind moves", () => {
  browser({ reduce: true });
  for (const kind of ["push", "pop", "modal", "replace", "none"] as const) expect(transitionTypes(kind)).toBe(false);
});

test("runTransition moves by the kind, and changes the screen at once without View Transition types", async () => {
  const started = browser();
  let updates = 0;
  await runTransition("push", () => void updates++);
  expect(started).toEqual([{ types: ["push"] }]);
  await runTransition("none", () => void updates++);
  expect(started).toHaveLength(1);
  const old = browser({ types: false });
  await runTransition("push", () => void updates++);
  expect(old).toHaveLength(0);
  expect(updates).toBe(3);
});

test("runTransition changes the screen at once outside a browser", async () => {
  let updated = false;
  await runTransition("push", () => {
    updated = true;
  });
  expect(updated).toBe(true);
});

test("a router moves by the kind of each navigation, and runs no View Transition where the browser has none", () => {
  const kindOf = (to: string): TransitionKind => (to === "/" ? "replace" : "push");
  expect(routerTransitions(kindOf)).toBe(false);
  browser();
  const transitions = routerTransitions(kindOf);
  if (transitions === false) throw new Error("no transitions in a browser with them");
  expect(transitions.types("/")).toEqual(["replace"]);
  expect(transitions.types("/post")).toEqual(["push"]);
});

test("a shared element is named apart from the frame's regions, as a CSS identifier", () => {
  expect(renderToStaticMarkup(<Shared name="avatar:1/2">x</Shared>)).toContain(
    "view-transition-name:shared-avatar_1_2",
  );
});

test("push and pop mirror in a right-to-left page, and reduced motion stops every View Transition", () => {
  expect(transitionRules[':root[dir="rtl"]']).toEqual({ "--my-direction": "-1" });
  expect(JSON.stringify(transitionRules["@keyframes my-push-in"])).toContain("var(--my-direction)");
  expect(transitionRules["@media (prefers-reduced-motion: reduce)"]).toEqual({
    "::view-transition-group(*), ::view-transition-old(*), ::view-transition-new(*)": { animation: "none !important" },
  });
});
