// SPDX-License-Identifier: AGPL-3.0-only

// The page's position on a change of screen (ADR-0025 §4): each route declares in its static data whether its screen
// brings back the position it was left at (`restore`), opens at the top (`top`), or brings it back only when the
// history brings the reader back to it, by the browser's Back and Forward, and opens at the top, its newest, on any
// other visit (`back`: the timelines and the reader's places that the navigation leads to). The design system keeps
// the positions, by the route and the path of the screen, and for `back` by the entry of the history it showed in too.
// Both come from the matches the router renders (screen.ts).

import { useScrollRestoration, type ScrollMode } from "@my/design-system";
import { useRouterState } from "@tanstack/react-router";
import { useState } from "react";
import { useShownScreen } from "./screen";

/** What a screen does with the page's position when it shows (the route's `scroll`). */
export type ScreenScroll = ScrollMode | "back";

/**
 * The key the position of `screen` shown at the history's entry `entry` is kept by, and what the design system does
 * with it: `back` keeps a position for each entry, of which a new entry has none, so that it opens at the top.
 */
export function screenScrollOf(screen: string, entry: string, scroll: ScreenScroll): { key: string; mode: ScrollMode } {
  return scroll === "back" ? { key: `${screen} ${entry}`, mode: "restore" } : { key: screen, mode: scroll };
}

/** Restores or resets the page's position as the shown screen declares, whenever the screen changes. */
export function useScreenScroll() {
  const screen = useShownScreen();
  const scroll = useRouterState({ select: (state) => state.matches.at(-1)?.staticData.scroll ?? "top" });
  // The entry the screen showed in: the router's location is the new one by the time its matches are, and it is
  // taken when the screen changes alone, so that a later change of the address on the same screen keeps its position.
  const entry = useRouterState({ select: (state) => state.location.state.__TSR_key ?? "" });
  const [shown, setShown] = useState({ screen, entry });
  if (shown.screen !== screen) setShown({ screen, entry });
  const { key, mode } = screenScrollOf(screen, shown.screen === screen ? shown.entry : entry, scroll);
  useScrollRestoration(key, mode);
}
