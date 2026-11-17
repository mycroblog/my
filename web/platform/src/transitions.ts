// SPDX-License-Identifier: AGPL-3.0-only

// View Transitions (ADR-0110 P2): whether the browser runs a View Transition with types (Chrome 125, Safari 18.2,
// Firefox 144), which the design system's motion of a change of screen needs. Below that, within the browser floor of
// ADR-0025 §1, the screen changes with no motion. The app asks here and gives the answer to the design system.

/** Whether the browser runs View Transitions with types. */
export function supportsViewTransitions(): boolean {
  return (
    typeof document !== "undefined" &&
    typeof document.startViewTransition === "function" &&
    typeof CSS !== "undefined" &&
    CSS.supports("selector(:active-view-transition-type(a))")
  );
}

const settled = new WeakSet<Document>();

/**
 * Handles the `ready` of every View Transition the page starts. A navigation that starts before the previous one's
 * transition has captured the old screen skips it, and Chromium rejects the skipped one's `ready` unhandled, although
 * the specification marks it handled; the router keeps only `updateCallbackDone`, so it cannot catch it. Call once
 * before the router's first navigation; a second call changes nothing.
 */
export function settleSkippedTransitions(): void {
  if (typeof document === "undefined" || typeof document.startViewTransition !== "function") return;
  if (settled.has(document)) return;
  settled.add(document);
  const start = document.startViewTransition.bind(document);
  document.startViewTransition = (options?: Parameters<Document["startViewTransition"]>[0]) => {
    const transition = start(options);
    transition.ready.catch(() => undefined);
    return transition;
  };
}
