// SPDX-License-Identifier: AGPL-3.0-only
// The height of the bar of a call that stays in view at the bottom of the page (`CallBar`, `MiniPlayer`, `CallDock`),
// as the `--call-bar-block-size` of the document, from which the app's stylesheet keeps focused elements clear of
// it (scroll-padding): the bar wraps on narrow screens, and a notice adds a row.
import { type RefObject, useEffect } from "react";

/** The custom property with the bar's height, for the app's scroll-padding. */
const blockSizeProperty = "--call-bar-block-size";

/** Writes the height of the element of `ref` to `--call-bar-block-size` while it is mounted (ResizeObserver). */
export function useCallBarBlockSize(ref: RefObject<HTMLElement | null>) {
  // The bar's height is an external fact that the document's stylesheet needs (react.dev, "Synchronizing with Effects").
  useEffect(() => {
    const element = ref.current;
    if (element === null) return;
    const root = document.documentElement;
    const write = () => {
      root.style.setProperty(blockSizeProperty, `${String(element.getBoundingClientRect().height)}px`);
    };
    write();
    const observer = typeof ResizeObserver === "function" ? new ResizeObserver(write) : null;
    observer?.observe(element);
    return () => {
      observer?.disconnect();
      root.style.removeProperty(blockSizeProperty);
    };
  }, [ref]);
}
