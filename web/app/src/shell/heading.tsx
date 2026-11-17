// SPDX-License-Identifier: AGPL-3.0-only

// The page-level accessibility of a screen (ADR-0025 §4, ADR-0110 §10): its h1, which names the document's title and
// takes the focus after a change of screen, so that a screen reader says where the reader went.

import { focusFrameHeading, FrameHeading } from "@my/design-system";
import { useEffect, useRef } from "react";
import { useShownScreen } from "./screen";

const headingId = "screen-heading";

/** The screen's h1, for assistive technology (the navigation shows where the reader is), and the document's title. */
export function ScreenHeading({ children }: { children: string }) {
  useEffect(() => {
    document.title = children;
  }, [children]);
  return <FrameHeading id={headingId}>{children}</FrameHeading>;
}

/**
 * Moves the focus to the new screen's h1 whenever the shown screen changes (screen.ts), once that screen is on the
 * page, not when the app opens, without moving the position the screen restored.
 */
export function useFocusOnNavigation() {
  const screen = useShownScreen();
  const shown = useRef(screen);
  useEffect(() => {
    if (shown.current === screen) return;
    shown.current = screen;
    focusFrameHeading(document);
  }, [screen]);
}
