// SPDX-License-Identifier: AGPL-3.0-only

// The kind of each navigation (ADR-0025 §3, §4): the design system moves the screen by it (routerTransitions), and
// the router asks for it on every change of its location.
//
// - The first screen, and a change of the search or the fragment alone, do not move (`none`).
// - Opening the composer over the screen raises it (`modal`); the screen under it stays. Closing it does not move.
// - Between the places of the navigation, siblings, the new place replaces the old (`replace`).
// - Elsewhere a navigation goes deeper (`push`), and going back in the history comes out again (`pop`).

import type { TransitionKind } from "@my/design-system";
import { composeTarget, parseComposeSearch } from "./shell/compose";
import { places } from "./shell/shell";

/** What the kind reads of a location of the router: its path, its search, and its index in the browser's history. */
type Location = { pathname: string; search?: Record<string, unknown> | undefined; state: { __TSR_index: number } };

/** A change of the router's location: from none on the first screen. */
export type NavigationChange = { fromLocation?: Location | undefined; toLocation: Location };

const isPlace = (pathname: string) => places.some((place) => place.href === pathname);
const composing = (location: Location) => composeTarget(parseComposeSearch(location.search ?? {})) !== undefined;

export function navigationKind({ fromLocation: from, toLocation: to }: NavigationChange): TransitionKind {
  if (from === undefined) return "none";
  if (!composing(from) && composing(to)) return "modal";
  if (from.pathname === to.pathname) return "none";
  if (isPlace(from.pathname) && isPlace(to.pathname)) return "replace";
  return to.state.__TSR_index < from.state.__TSR_index ? "pop" : "push";
}
