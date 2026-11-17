// SPDX-License-Identifier: AGPL-3.0-only

// The screen the router shows, which the page's position (scroll.ts) and the focus (heading.tsx) follow. It comes
// from the matches the router renders, not from its location: the location changes as a navigation starts, while the
// old screen is still on the page, and the matches change in the commit that puts the new screen there.

import { useRouterState } from "@tanstack/react-router";

/** What the shown screen is read from: the router's state, of which only the matches it renders. */
type Shown = { matches: readonly { routeId: string; pathname: string }[] };

/**
 * The shown screen, as its route and its path. An address with no screen matches the root alone, whose path is "/":
 * its route tells it from the home screen.
 */
export function shownScreen({ matches }: Shown): string {
  const leaf = matches.at(-1);
  return leaf ? `${leaf.routeId} ${leaf.pathname}` : "";
}

/** The shown screen (shownScreen), which changes once the new screen is on the page. */
export const useShownScreen = () => useRouterState({ select: shownScreen });
