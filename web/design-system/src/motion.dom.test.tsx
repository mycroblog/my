// SPDX-License-Identifier: AGPL-3.0-only

// A change of screen in a document. The simulation runs no View Transition, so the document's startViewTransition is
// a stand-in here that records what it was asked and changes the screen: that the screens then move, mirrored in a
// right-to-left page, is a browser's to show.

import { afterEach, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { routerTransitions, runTransition, Shared, transitionTypes, type TransitionKind } from "./motion";

type Asked = { types: string[] };
type Transitioning = { startViewTransition?: (options: { update: () => void; types: string[] }) => unknown };

/** Gives the document View Transitions with types, and the transitions it was asked for, in order. */
function viewTransitions() {
  const asked: Asked[] = [];
  (document as Transitioning).startViewTransition = ({ update, types }) => {
    asked.push({ types });
    update();
    return { updateCallbackDone: Promise.resolve() };
  };
  return asked;
}

/** The reader's motion preference, as happy-dom's media queries answer it. */
function reduceMotion(reduce: boolean) {
  const { happyDOM } = window as unknown as { happyDOM: { settings: { device: { prefersReducedMotion: string } } } };
  happyDOM.settings.device.prefersReducedMotion = reduce ? "reduce" : "no-preference";
}

afterEach(() => {
  delete (document as Transitioning).startViewTransition;
  reduceMotion(false);
});

const kinds = ["push", "pop", "modal", "replace"] as const;

test("each kind that moves is the type of its View Transition; none changes the screen at once", () => {
  for (const kind of kinds) expect(transitionTypes(kind)).toEqual([kind]);
  expect(transitionTypes("none")).toBe(false);
});

test("with reduced motion no kind moves", () => {
  reduceMotion(true);

  for (const kind of [...kinds, "none"] as const) expect(transitionTypes(kind)).toBe(false);
});

test("runTransition asks the document for a View Transition of the kind, which changes the screen", async () => {
  const asked = viewTransitions();
  let updates = 0;

  await runTransition("push", () => void updates++, true);

  expect(asked).toEqual([{ types: ["push"] }]);
  expect(updates).toBe(1);
});

test("runTransition changes the screen at once for no kind, with reduced motion, and where the browser has no types", async () => {
  const asked = viewTransitions();
  let updates = 0;

  await runTransition("none", () => void updates++, true);
  await runTransition("push", () => void updates++, false);
  reduceMotion(true);
  await runTransition("push", () => void updates++, true);

  expect(asked).toEqual([]);
  expect(updates).toBe(3);
});

test("a router moves by the kind of each navigation", () => {
  const kindOf = (to: string): TransitionKind => (to === "/" ? "replace" : "push");

  const transitions = routerTransitions(kindOf, true);

  if (transitions === false) throw new Error("no transitions in a browser with them");
  expect(transitions.types("/")).toEqual(["replace"]);
  expect(transitions.types("/post")).toEqual(["push"]);
});

test("a shared element is named for its View Transition apart from the frame's regions, as a CSS identifier", () => {
  render(
    <Shared name="avatar:1/2">
      <img alt="Mika" src="https://m.example/mika.webp" />
    </Shared>,
  );

  const named = screen.getByRole("img", { name: "Mika" }).parentElement;
  expect(named?.style.getPropertyValue("view-transition-name")).toBe("shared-avatar_1_2");
});
