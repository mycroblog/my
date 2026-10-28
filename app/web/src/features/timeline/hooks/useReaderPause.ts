// SPDX-License-Identifier: AGPL-3.0-only
import { type RefObject, useEffect, useState } from "react";

/**
 * Whether the reader is in the feed: focus is inside it, or text in it is selected. Content that moves on its own
 * must be pausable (WCAG 2.2.2), so the home timeline does not insert new posts meanwhile (ADR-0077 §4).
 * Focus that moves to a dialog or any element outside the feed ends the pause. The browser's focus and selection
 * are an external system, which an effect subscribes to (react.dev: "You Might Not Need an Effect").
 */
export function useReaderInFeed(feedRef: RefObject<HTMLElement | null>): boolean {
  const [focused, setFocused] = useState(false);
  const [selecting, setSelecting] = useState(false);
  useEffect(() => {
    const inFeed = (node: EventTarget | Node | null) =>
      node instanceof Node && feedRef.current?.contains(node) === true;
    const onFocusIn = (event: FocusEvent) => {
      setFocused(inFeed(event.target));
    };
    // During a focus move the active element is the body, so the element receiving focus decides.
    const onFocusOut = (event: FocusEvent) => {
      setFocused(inFeed(event.relatedTarget));
    };
    const onSelectionChange = () => {
      const selection = document.getSelection();
      setSelecting(
        selection !== null && !selection.isCollapsed && inFeed(selection.anchorNode) && inFeed(selection.focusNode),
      );
    };
    document.addEventListener("focusin", onFocusIn);
    document.addEventListener("focusout", onFocusOut);
    document.addEventListener("selectionchange", onSelectionChange);
    return () => {
      document.removeEventListener("focusin", onFocusIn);
      document.removeEventListener("focusout", onFocusOut);
      document.removeEventListener("selectionchange", onSelectionChange);
    };
  }, [feedRef]);
  return focused || selecting;
}
