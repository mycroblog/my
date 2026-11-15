// SPDX-License-Identifier: AGPL-3.0-only

// A state shown at rest, for the components' stories (ADR-0165 §1): hover, press and keyboard focus follow the
// pointer and the keyboard, so a page that shows them side by side asks for them here; and a stage for the motion of
// a change of screen; and a wait until a modal has come in, so that a story's checks read it at rest. Only the subpath
// @my/design-system/preview exports it; screens never use it.

import { createContext, useContext, type ReactNode } from "react";

export type PreviewState = "hover" | "pressed" | "focus-visible";

const PreviewContext = createContext<PreviewState | undefined>(undefined);

/** The state a component shows, the real ones aside. */
export const usePreviewState = () => useContext(PreviewContext);

/** Shows its components in `state`. */
export function Preview({ state, children }: { state: PreviewState | undefined; children: ReactNode }) {
  return <PreviewContext value={state}>{children}</PreviewContext>;
}

const ItemsContext = createContext<Readonly<Record<string, PreviewState>>>({});

/** The state an item of a collection (a menu's) shows, by its id, the real ones aside. */
export const usePreviewItemState = (id: string) => useContext(ItemsContext)[id];

/**
 * Shows the items of the collections inside by their ids in a state each, so that a menu shows one item hovered and
 * another focused from the keyboard.
 */
export function PreviewItems({
  states,
  children,
}: {
  states: Readonly<Record<string, PreviewState>>;
  children: ReactNode;
}) {
  return <ItemsContext value={states}>{children}</ItemsContext>;
}

const ExpandedContext = createContext(false);

/** Whether the disclosures inside show what they fold at first (a content warning opened). */
export const usePreviewExpanded = () => useContext(ExpandedContext);

/** Shows the disclosures inside expanded at first, so that a page shows a post's content warning open beside it folded. */
export function PreviewExpanded({ children }: { children: ReactNode }) {
  return <ExpandedContext value>{children}</ExpandedContext>;
}

/**
 * The screen a change of screen moves (motion.tsx) outside the app's frame, whose main column it is there: the stage
 * on which a catalog runs each kind between sample screens. One on a page.
 */
export function TransitionStage({ children }: { children: ReactNode }) {
  return <div className="[view-transition-name:screen]">{children}</div>;
}

/**
 * Resolves once the open dialog has come in: it is on the page, and neither it nor what holds it is still fading or
 * moving in (a modal's entrance, modal.tsx), so that what a check reads is the dialog at rest.
 */
export async function modalShown(timeout = 3000): Promise<void> {
  const settled = () => {
    const dialog = document.querySelector('[role="dialog"]');
    if (dialog === null) return false;
    for (let el: Element | null = dialog; el !== null; el = el.parentElement) {
      const style = getComputedStyle(el);
      if (style.opacity !== "1" || (style.transform !== "none" && style.transform !== "")) return false;
    }
    return document.getAnimations().every((a) => a.playState !== "running");
  };
  const end = Date.now() + timeout;
  while (!settled()) {
    if (Date.now() > end) throw new Error("the dialog did not come in");
    await new Promise((done) => requestAnimationFrame(done));
  }
}
