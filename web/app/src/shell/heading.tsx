// SPDX-License-Identifier: AGPL-3.0-only

// The page-level accessibility of a screen (ADR-0025 §4, ADR-0110 §10): its h1, which names the document's title and
// takes the focus after a change of screen, so that a screen reader says where the reader went.

import { FrameHeading } from "@my/design-system";
import { useEffect, useRef } from "react";

const headingId = "screen-heading";

/** The screen's h1, for assistive technology (the navigation shows where the reader is), and the document's title. */
export function ScreenHeading({ children }: { children: string }) {
  useEffect(() => {
    document.title = children;
  }, [children]);
  return <FrameHeading id={headingId}>{children}</FrameHeading>;
}

/** Moves the focus to the new screen's h1 whenever `pathname` changes, not when the app opens. */
export function useFocusOnNavigation(pathname: string) {
  const shown = useRef(pathname);
  useEffect(() => {
    if (shown.current === pathname) return;
    shown.current = pathname;
    document.getElementById(headingId)?.focus();
  }, [pathname]);
}
