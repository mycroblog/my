// SPDX-License-Identifier: AGPL-3.0-only

// A state shown at rest, for a catalog of the components (ADR-0168): hover, press and keyboard focus follow the
// pointer and the keyboard, so a page that shows them side by side asks for them here. Only the subpath
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
