// SPDX-License-Identifier: AGPL-3.0-only
// The height of a bar that stays in view, as a custom property of the document, from which the stylesheets keep the
// rest of the screen clear of it: the call bar at the bottom (`--call-bar-block-size`, for the app's scroll-padding),
// the offline bar at the top (`--offline-bar-block-size`, for the heights of the shell), the signed-out top bar
// (`--top-bar-block-size`) and the progress line of a page of sections (`--section-progress-block-size`), under which
// what sticks to the top of the page starts and to which a heading scrolls. A bar wraps on narrow
// screens and grows with its content, so its height is measured (ResizeObserver), never assumed.
import { type RefObject, useEffect } from "react";

/**
 * Writes the height of the element of `ref` to the document's custom property `property` while it is mounted; nothing
 * while `property` is undefined (an element that only sometimes is such a bar).
 */
export function useBlockSizeProperty(ref: RefObject<HTMLElement | null>, property: `--${string}` | undefined) {
  // The bar's height is an external fact that the document's stylesheet needs (react.dev, "Synchronizing with Effects").
  useEffect(() => {
    const element = ref.current;
    if (element === null || property === undefined) return;
    const root = document.documentElement;
    const write = () => {
      root.style.setProperty(property, `${String(element.getBoundingClientRect().height)}px`);
    };
    write();
    const observer = typeof ResizeObserver === "function" ? new ResizeObserver(write) : null;
    observer?.observe(element);
    return () => {
      observer?.disconnect();
      root.style.removeProperty(property);
    };
  }, [ref, property]);
}
