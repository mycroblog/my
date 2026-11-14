// SPDX-License-Identifier: AGPL-3.0-only

// The page's position on a change of screen (ADR-0025 §4): each route declares in its static data whether its screen
// brings back the position it was left at (`restore`) or opens at the top (`top`), and the design system keeps the
// positions, by the route and the path of the screen. Both come from the matches the router renders, not from its
// location, which changes before the screen does.

import { useScrollRestoration } from "@my/design-system";
import { useRouterState } from "@tanstack/react-router";

/** Restores or resets the page's position as the shown screen declares, whenever the screen changes. */
export function useScreenScroll() {
  // An address with no screen matches the root alone, whose path is "/": its route tells it from the home screen.
  const screen = useRouterState({
    select: (state) => {
      const leaf = state.matches.at(-1);
      return leaf ? `${leaf.routeId} ${leaf.pathname}` : "";
    },
  });
  const mode = useRouterState({ select: (state) => state.matches.at(-1)?.staticData.scroll ?? "top" });
  useScrollRestoration(screen, mode);
}
