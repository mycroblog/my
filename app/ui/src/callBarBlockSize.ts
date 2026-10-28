// SPDX-License-Identifier: AGPL-3.0-only
// The height of the bar of a call that stays in view at the bottom of the page (`CallBar`, `MiniPlayer`, `CallDock`),
// as the `--call-bar-block-size` of the document, from which the app's stylesheet keeps focused elements clear of
// it (scroll-padding): the bar wraps on narrow screens, and a notice adds a row.
import type { RefObject } from "react";
import { useBlockSizeProperty } from "./blockSizeProperty";

/** Writes the height of the element of `ref` to `--call-bar-block-size` while it is mounted (ResizeObserver). */
export function useCallBarBlockSize(ref: RefObject<HTMLElement | null>) {
  useBlockSizeProperty(ref, "--call-bar-block-size");
}
