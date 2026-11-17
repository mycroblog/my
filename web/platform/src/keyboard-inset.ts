// SPDX-License-Identifier: AGPL-3.0-only

// The height the software keyboard covers at the bottom of the layout viewport, which the browsers tell differently:
// Chrome on Android resizes the viewport itself (the page's `interactive-widget=resizes-content`), so nothing is
// covered; a browser that overlays the keyboard with the Virtual Keyboard API gives `env(keyboard-inset-height)` to
// CSS; Safari on iOS does neither and only shrinks the visual viewport, whose loss is measured here. The app hands the
// height to the design system's composer, which keeps its row with Post right above the keyboard.

type ViewportLike = Pick<VisualViewport, "height" | "offsetTop" | "addEventListener" | "removeEventListener">;

/** The height covered under the visual viewport, in whole px; 0 where nothing is. */
export function coveredHeight(layoutHeight: number, viewport: Pick<VisualViewport, "height" | "offsetTop">): number {
  return Math.max(0, Math.round(layoutHeight - viewport.height - viewport.offsetTop));
}

/**
 * Calls `onInset` with the height the keyboard covers now and whenever it changes, until the function it returns is
 * called. Without a visual viewport (an old browser, a server) it never calls.
 */
export function watchKeyboardInset(
  onInset: (px: number) => void,
  env: { viewport?: ViewportLike | null | undefined; layoutHeight?: () => number } = {},
): () => void {
  const viewport = "viewport" in env ? env.viewport : typeof visualViewport === "undefined" ? null : visualViewport;
  if (!viewport) return () => undefined;
  const layoutHeight = env.layoutHeight ?? (() => window.innerHeight);
  let last = -1;
  const measure = () => {
    const inset = coveredHeight(layoutHeight(), viewport);
    if (inset === last) return;
    last = inset;
    onInset(inset);
  };
  measure();
  viewport.addEventListener("resize", measure);
  viewport.addEventListener("scroll", measure);
  return () => {
    viewport.removeEventListener("resize", measure);
    viewport.removeEventListener("scroll", measure);
  };
}
