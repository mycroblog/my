// SPDX-License-Identifier: AGPL-3.0-only

// The router (ADR-0025 §1, ADR-0028 §6): file-based routes from src/routes, compiled into routeTree.gen.ts by the
// TanStack Router plugin of vite.config.ts, and by scripts/routes.ts before tsc, the lint and the tests. TanStack
// Query is the one cache of server data, so the router keeps none of its own.

import type { AppData } from "@my/app-data";
import { routerTransitions } from "@my/design-system";
import type { ScrollMode } from "@my/design-system";
import { supportsViewTransitions } from "@my/platform";
import { createMemoryHistory, createRouter } from "@tanstack/react-router";
import type { RuntimeConfig } from "./config";
import { routeTree } from "./routeTree.gen";
import { navigationKind } from "./transitions";

/**
 * What every route reads: the session's data (null without a session), the instance's configuration, and whether a
 * screen is in place yet (the first load has ended).
 */
export type RouterContext = { data: AppData | null; config: RuntimeConfig; shown: () => boolean };

export type AppRouterOptions = Omit<RouterContext, "shown"> & {
  /**
   * The path to start at in a history of the router's own, apart from the page's address (a screen's story);
   * without it the router follows the browser's history.
   */
  at?: string;
};

export function createAppRouter({ data, config, at }: AppRouterOptions) {
  let shown = false;
  const router = createRouter({
    routeTree,
    context: { data, config, shown: () => shown },
    ...(at === undefined ? {} : { history: createMemoryHistory({ initialEntries: [at] }) }),
    // Every match is stale at once, so that a screen entered again runs its loader, which returns what the cache holds
    // unless a write marked it stale. Not 0: a wall clock that steps back would make a match look fresh.
    defaultStaleTime: Number.NEGATIVE_INFINITY,
    defaultPreloadStaleTime: Number.NEGATIVE_INFINITY,
    // A screen visited before renders once its loader has ended, not from its previous match while the loader runs.
    defaultStaleReloadMode: "blocking",
    // Every navigation moves the screen by its kind (transitions.ts).
    defaultViewTransition: routerTransitions(navigationKind, supportsViewTransitions()),
    // The page's position is the screens' (shell/scroll.ts): the router neither restores it nor resets it.
    scrollRestoration: () => false,
  });
  // From the end of the first load on, a screen's loader waits for its data (screen-load.ts).
  const unsubscribe = router.subscribe("onResolved", () => {
    unsubscribe();
    shown = true;
  });
  return router;
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
  // What every route declares about itself.
  interface StaticDataRouteOption {
    /** Whether the screen brings back the position it was left at, or opens at the top (ADR-0025 §4). */
    scroll: ScrollMode;
  }
}
