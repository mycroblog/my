// SPDX-License-Identifier: AGPL-3.0-only
// After a story has rendered and passed axe, moves the pointer off its frame, so that the next story does not start
// under a pointer, and checks that nothing in the frame still animates with prefers-reduced-motion: reduce: every
// animation sits under motion-safe: (web-client/README.md D6). Stories render and run play functions without the
// emulation.
// The Playwright provider types cdp() with Chromium's DevTools protocol.
import type {} from "@vitest/browser-playwright";
import { afterEach, expect } from "vitest";
import { cdp, userEvent } from "vitest/browser";

/** Sets Chromium's prefers-reduced-motion to reduce, or back to the browser's default. */
async function emulateReducedMotion(reduce: boolean) {
  await cdp().send("Emulation.setEmulatedMedia", {
    features: [{ name: "prefers-reduced-motion", value: reduce ? "reduce" : "" }],
  });
}

/** The hovered controls: React Aria's hover state, and CSS :hover on what a reader can press or fill in. */
const hoveredControls = [
  "[data-hovered]",
  ":is(a, button, input, select, textarea, summary, label):hover",
  ":is([role=button], [role=link], [role=menuitem], [role=option], [role=tab], [role=checkbox], [role=radio], [role=switch], [role=slider]):hover",
].join(", ");

/**
 * Moves the pointer off the frame and waits until no control in it is hovered. A play function leaves the pointer on the
 * control it pressed, where the next story's controls would render under it. userEvent goes through the test runner,
 * which knows where the story's iframe sits in the page; the pointer moves to a spot fixed at the viewport's inline-end
 * bottom corner.
 */
async function moveThePointerAway(frame: HTMLElement) {
  const spot = document.createElement("div");
  const spotStyle: Record<string, string> = {
    position: "fixed",
    "inset-inline-end": "0",
    "inset-block-end": "0",
    "inline-size": "2px",
    "block-size": "2px",
  };
  for (const [property, value] of Object.entries(spotStyle)) spot.style.setProperty(property, value);
  document.body.append(spot);
  try {
    // An open modal's backdrop covers the spot, so skip the actionability checks: only the pointer has to move.
    await userEvent.hover(spot, { force: true });
  } finally {
    spot.remove();
  }
  // A control that moved from under a still pointer (a message appeared above it) gets no pointer event from the
  // browser, so React Aria never hears that the pointer left; tell it the way the browser would have.
  for (const element of frame.querySelectorAll<HTMLElement>("[data-hovered]")) {
    element.dispatchEvent(
      new PointerEvent("pointerout", { bubbles: true, pointerType: "mouse", relatedTarget: document.body }),
    );
  }
  await expect
    // Only controls change their look on hover; a sheet that fills a phone viewport is under the pointer wherever it goes.
    .poll(() => [...frame.querySelectorAll(hoveredControls)].map((element) => element.outerHTML.slice(0, 120)))
    .toEqual([]);
}

/** Names a running animation for the failure message: the CSS animation or property and its element. */
function describeAnimation(animation: Animation): string {
  const name =
    animation instanceof CSSAnimation
      ? animation.animationName
      : animation instanceof CSSTransition
        ? animation.transitionProperty
        : animation.id;
  const target = animation.effect instanceof KeyframeEffect ? animation.effect.target : null;
  return `${name} on ${target?.outerHTML.slice(0, 120) ?? "an unknown element"}`;
}

afterEach(async (context) => {
  // Only story tests carry a composed story; it stays mounted until the next one renders.
  const { story } = context as typeof context & { story?: { id: string } };
  if (story === undefined || context.task.result?.state === "fail") return;
  const frame = document.querySelector<HTMLElement>("[data-story-frame]");
  if (frame === null)
    throw new Error(`story ${story.id} rendered no [data-story-frame]; is the preview decorator missing?`);
  await emulateReducedMotion(true);
  try {
    await moveThePointerAway(frame);
    // Transitions already in flight (such as the color change when the preview applies the theme)
    // run to their end even after the media change, so wait for them; none can start under reduce.
    const inFlight = frame.getAnimations({ subtree: true }).filter((animation) => animation instanceof CSSTransition);
    await Promise.allSettled(inFlight.map((transition) => transition.finished));
    const running = frame
      .getAnimations({ subtree: true })
      .filter((animation) => animation.playState === "running")
      .map(describeAnimation);
    expect(running, `story ${story.id} still animates with prefers-reduced-motion: reduce`).toEqual([]);
  } finally {
    await emulateReducedMotion(false);
  }
});
