// SPDX-License-Identifier: AGPL-3.0-only
// The root route: the app layout around every page, and the error screen for a failure of the layout
// itself. Pages fail inside the layout instead, through the router's defaultErrorComponent (router.ts).
import { createRootRouteWithContext } from "@tanstack/react-router";
import { viewerQueryOptions } from "../features/session";
import { type RouterContext, shellQueryRetry } from "../lib/query";
import { AppLayout } from "./-layout/AppLayout";
import { NotFound, RootError } from "./-layout/RouteStates";

export const Route = createRootRouteWithContext<RouterContext>()({
  // A signed-in page renders once the viewer's account has loaded, so that the navigation has the staff areas the
  // viewer's permissions allow from its first render instead of gaining them after it. The page's own
  // beforeLoad has resumed the session by now. A failure does not fail the page: the shell reads the account
  // again and shows the failure where it uses it (web-client/09-app-shell.md, the shell's loading).
  loader: async ({ context: { queryClient, api, session } }) => {
    if (session.status !== "signed-in") return;
    try {
      // A cached account is used at once; the shell refetches it when it is stale. It retries as the shell's
      // query does, which then does not send a request that failed here again on mount (shellQueryReader).
      await queryClient.query({ ...viewerQueryOptions(api), staleTime: Infinity, retry: shellQueryRetry });
    } catch {
      // The shell's own query handles it.
    }
  },
  component: AppLayout,
  errorComponent: RootError,
  notFoundComponent: NotFound,
});
