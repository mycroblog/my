// SPDX-License-Identifier: AGPL-3.0-only

// The kinds of change of screen and their motion (ADR-0025 §4, ADR-0110 P2): every change of screen is one of five
// kinds, which the app chooses for each navigation; the design system moves the screen by the kind with a View
// Transition (motion-css.ts holds the motion), and moves the shared elements a feature names from their place on the
// old screen to their place on the new one.
//
// - `push`: deeper, to what the reader opened; `pop`: back out of it. They mirror in a right-to-left page.
// - `modal`: a screen raised over the one the reader was on.
// - `replace`: a sibling in place of the screen, as between the places of the navigation.
// - `none`: no motion, as for the first screen or a change of the address alone.
//
// A browser without View Transitions and their types, or a reader who asks for reduced motion, sees the new screen at
// once, with no motion.

import type { ReactNode } from "react";

export type TransitionKind = "push" | "pop" | "modal" | "replace" | "none";

export const transitionKinds: readonly TransitionKind[] = ["push", "pop", "modal", "replace", "none"];

// Whether the browser runs a View Transition with types (Chrome 125, Safari 18.2, Firefox 144), which the motion of a
// kind needs. Below that, within the browser floor of ADR-0025 §1, the screen changes with no motion. The one place
// that detects it.
const supportsTransitions = () =>
  typeof document !== "undefined" &&
  typeof document.startViewTransition === "function" &&
  typeof CSS !== "undefined" &&
  CSS.supports("selector(:active-view-transition-type(a))");

const reducedMotion = () => typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;

/** The View Transition types of a change of screen of `kind`, or false when the screen changes with no motion. */
export function transitionTypes(kind: TransitionKind): string[] | false {
  return kind === "none" || reducedMotion() ? false : [kind];
}

/**
 * A router's View Transitions (TanStack Router's `defaultViewTransition`): each navigation moves by the kind
 * `kindOf` gives it; false where the browser runs none, so that the router changes the screen at once.
 */
export function routerTransitions<Change>(
  kindOf: (change: Change) => TransitionKind,
): false | { types: (change: Change) => string[] | false } {
  return supportsTransitions() && { types: (change: Change) => transitionTypes(kindOf(change)) };
}

/**
 * Changes the screen with `update`, which changes the DOM before it returns (in React, inside `flushSync`), moving it
 * by `kind`. It resolves once the new screen is in the DOM.
 */
export async function runTransition(kind: TransitionKind, update: () => void): Promise<void> {
  const types = supportsTransitions() && transitionTypes(kind);
  if (types === false) {
    update();
    return;
  }
  await document.startViewTransition({ update, types }).updateCallbackDone;
}

/** The View Transition name of a shared element, a CSS identifier apart from the names of the frame's regions. */
const sharedName = (name: string) => `shared-${name.replace(/[^\w-]/g, "_")}`;

/**
 * Names what it holds as a shared element: on a change of screen it moves from its place on the old screen to the
 * place of the element of the same name on the new one. A name is on one element of a screen at a time (a post's
 * avatar by the post's id).
 */
export function Shared({ name, children }: { name: string; children: ReactNode }) {
  return (
    <span className="inline-flex" style={{ viewTransitionName: sharedName(name) }}>
      {children}
    </span>
  );
}
