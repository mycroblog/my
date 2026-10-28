// SPDX-License-Identifier: AGPL-3.0-only
// Visual snapshots of every story (ADR-0027) not tagged `no-screenshot`. After a story has rendered and passed axe, its frame
// is captured at each container width; the project fixes the direction and theme (../vitest.config.ts). A story of the
// app shell sets a viewport instead (src/storyViewports.ts), which @storybook/addon-vitest has applied; it is captured
// once, at that viewport.
// Baselines live in src/__screenshots__/ and are generated only in the pinned Playwright image
// (`scripts/task.sh ui:snapshots-update`), because fonts and anti-aliasing differ between machines.
// The capture runs with prefers-reduced-motion: reduce, and first checks that nothing in the frame
// still animates then: every animation sits under motion-safe: (web-client/README.md D6), and a spinner
// cannot keep the screenshot from settling. Stories render and run play functions without it. A story tagged
// `forced-colors` is captured in forced-colors mode too.
// The Playwright provider types cdp() with Chromium's DevTools protocol.
import type {} from "@vitest/browser-playwright";
import { afterEach, expect } from "vitest";
import { cdp, page, userEvent } from "vitest/browser";

/** Container widths in CSS px: a phone column, a tablet column and a desktop main column. */
const widths = [320, 640, 1024] as const;
/** The viewport @storybook/addon-vitest gives a story without a viewport parameter. */
const viewportWidth = 1200;
const viewportHeight = 900;

/**
 * Sets Chromium's prefers-reduced-motion to reduce and, for a story tagged `forced-colors`, turns forced-colors mode on
 * (the look of Windows contrast themes); or both back to the browser's default.
 */
async function emulateCaptureMedia(capture: boolean, forcedColors = false) {
  await cdp().send("Emulation.setEmulatedMedia", {
    features: [
      { name: "prefers-reduced-motion", value: capture ? "reduce" : "" },
      { name: "forced-colors", value: capture && forcedColors ? "active" : "" },
    ],
  });
}

/**
 * Moves the pointer off the frame and waits until no control in it is hovered. A play function leaves the pointer on the
 * control it pressed, and an earlier story leaves it wherever it was, so a control would be captured in its hover look,
 * or not, depending on timing. userEvent goes through the test runner, which knows where the story's iframe sits in the
 * page; the pointer moves to a spot fixed at the viewport's inline-end bottom corner, outside every frame width.
 */
/** The hovered controls: React Aria's hover state, and CSS :hover on what a reader can press or fill in. */
const hoveredControls = [
  "[data-hovered]",
  ":is(a, button, input, select, textarea, summary, label):hover",
  ":is([role=button], [role=link], [role=menuitem], [role=option], [role=tab], [role=checkbox], [role=radio], [role=switch], [role=slider]):hover",
].join(", ");

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

/** The named viewport a story sets (the `viewport` global of Storybook), if any. */
function viewportOf(globals: Record<string, unknown>): string | undefined {
  const viewport = globals["viewport"];
  if (typeof viewport !== "object" || viewport === null || !("value" in viewport)) return undefined;
  return typeof viewport.value === "string" && viewport.value !== "" ? viewport.value : undefined;
}

afterEach(async (context) => {
  // Only story tests carry a composed story; it stays mounted until the next one renders.
  const { story } = context as typeof context & {
    story?: { id: string; tags: readonly string[]; globals: Record<string, unknown> };
  };
  if (story === undefined || context.task.result?.state === "fail") return;
  // A story tagged `no-screenshot` exists for its play function (a long, virtualized list); it still
  // renders and passes axe, and the same component's other stories carry its screenshots.
  if (story.tags.includes("no-screenshot")) return;
  const frame = document.querySelector<HTMLElement>("[data-story-frame]");
  if (frame === null)
    throw new Error(`story ${story.id} rendered no [data-story-frame]; is the preview decorator missing?`);
  const { locale, theme } = story.globals;
  await emulateCaptureMedia(true, story.tags.includes("forced-colors"));
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
    const viewport = viewportOf(story.globals);
    if (viewport !== undefined) {
      await expect
        .element(page.elementLocator(frame))
        .toMatchScreenshot(`${story.id}-${String(locale)}-${String(theme)}-${viewport}`);
      return;
    }
    for (const width of widths) {
      frame.style.inlineSize = `${width}px`;
      // Open popovers and tooltips are positioned against their trigger, and React Aria repositions them on
      // window resize; the trigger moves with the frame width (in RTL it sits at the inline end).
      window.dispatchEvent(new Event("resize"));
      // The tester page paints only what fits its viewport, so it grows to the whole story. The width stays
      // the 1200 px that the Storybook addon sets; the container width is what the components answer to.
      const bottom = Math.ceil(frame.getBoundingClientRect().bottom + window.scrollY);
      await page.viewport(viewportWidth, Math.max(viewportHeight, bottom + 16));
      await expect
        .element(page.elementLocator(frame))
        .toMatchScreenshot(`${story.id}-${String(locale)}-${String(theme)}-${width}`);
    }
  } finally {
    frame.style.inlineSize = "";
    await emulateCaptureMedia(false);
  }
});
