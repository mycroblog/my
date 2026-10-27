// SPDX-License-Identifier: AGPL-3.0-only
// The router (ADR-0025): file-based routes from src/routes, compiled into routeTree.gen.ts by the
// TanStack Router Vite plugin, and by scripts/routes.ts before tsc and the tests. Navigations commit inside
// React's startTransition (the router's Transitioner), so the current page stays on screen while the next one
// loads (web-client/README.md E4); the pending component appears only when a load takes longer than defaultPendingMs.
import { createRouter, type RouterHistory } from "@tanstack/react-router";
import type { ErrorReporter } from "./lib/errorReporting";
import type { PlainClientMessageKey } from "./lib/i18n";
import type { RouterContext } from "./lib/query";
import { routeTree } from "./routeTree.gen";
import { RouteError, RoutePending } from "./routes/-layout/RouteStates";

/** The options of the app's router other than its routes, history and context. */
export const routerOptions = {
  // Every page that fails renders this inside the layout (web-client/README.md E1); the root route has its own
  // error component for a failure of the layout (routes/__root.tsx).
  defaultErrorComponent: RouteError,
  defaultPendingComponent: RoutePending,
  // Start loading a page's code and data when the pointer rests on or touches a link to it.
  defaultPreload: "intent",
  // TanStack Query caches the data (web-client/README.md S1), so the router keeps no data of its own: every match is
  // stale at once, and a page entered again runs its loader, which returns what the cache holds unless a write marked
  // it stale. Not 0: the router compares the match's age with Date.now(), and a wall clock that steps back (an NTP
  // correction, a virtual machine syncing with its host) would make a match look fresh and skip that loader.
  defaultStaleTime: Number.NEGATIVE_INFINITY,
  defaultPreloadStaleTime: Number.NEGATIVE_INFINITY,
  // A page visited before renders only once its loader has ended, not from its previous match while the loader
  // runs (TanStack Router's default "background"): a loader returns at once what is cached and not marked
  // stale, and fetches again what a write has marked, so that the page never shows what it was before the
  // write (web-client/README.md S7, ADR-0045).
  defaultStaleReloadMode: "blocking",
  scrollRestoration: true,
  // A remote account's profile is /@alice@example.com, as the server's shell routes have it (ADR-0028);
  // the `@` of a path param stays as it is instead of becoming %40.
  pathParamsAllowedCharacters: ["@"],
} as const;

export type AppRouterOptions = RouterContext & {
  /** The browser's history by default; tests pass a memory history. */
  history?: RouterHistory;
  /** Where the errors of the pages are reported (lib/errorReporting); a router without one reports nothing. */
  errors?: ErrorReporter;
};

export function createAppRouter({ queryClient, api, session, history, errors }: AppRouterOptions) {
  const router = createRouter({
    ...routerOptions,
    routeTree,
    context: { queryClient, api, session },
    ...(history === undefined ? {} : { history }),
    // Every page that fails is reported from here (ADR-0103 §6). A route's own `onCatch` replaces this one, so a
    // route that has one calls `errors.report(error, "route_error")` itself.
    ...(errors !== undefined && {
      defaultOnCatch: (error: unknown) => {
        errors.report(error, "route_error");
      },
    }),
  });
  errors?.attachRouter(router);
  return router;
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
  // What a route tells the app shell about itself, read from the current matches (TanStack Router's
  // static route data).
  interface StaticDataRouteOption {
    /** The shell floats the button that opens the new-post sheet over this page (timelines and profiles; web-client/16-look-states-and-wording.md G4). */
    composeButton?: boolean;
    /**
     * The page's h1 only says where the user is, which the navigation already shows (an item of the navigation
     * with nothing beside its heading): PageHeading hides it visually and keeps it for assistive technology, on
     * the page and on its pending and error states (web-client/README.md Y5).
     */
    hideHeading?: boolean;
    /** The shell gives the page a wide column (the signed-out landing, whose hero has posts beside its text). */
    wideMain?: boolean;
    /**
     * The page's name, the text of its h1. RouteError keeps it as the heading when the page fails, so that the
     * page and the document title stay named after the page (web-client/16-look-states-and-wording.md G16). A page whose name depends on
     * its params passes the heading to RouteError from its own errorComponent instead.
     */
    title?: PlainClientMessageKey;
  }
}
