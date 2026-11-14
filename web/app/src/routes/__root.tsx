// SPDX-License-Identifier: AGPL-3.0-only

// The root of every screen: the app's shell around the screen's route, with the links of the design system's parts
// navigating through the router. An address the app has no screen for shows its own heading inside the shell.

import { FeedNotice, NavigationProvider } from "@my/design-system";
import { createRootRouteWithContext, Outlet, useRouter, useRouterState } from "@tanstack/react-router";
import { useStrings } from "../locale";
import type { RouterContext } from "../router";
import { ScreenHeading, useFocusOnNavigation } from "../shell/heading";
import { Shell } from "../shell/shell";

export const Route = createRootRouteWithContext<RouterContext>()({ component: Root, notFoundComponent: NotFound });

function Root() {
  const router = useRouter();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const { config } = Route.useRouteContext();
  useFocusOnNavigation(pathname);
  return (
    <NavigationProvider
      navigate={(href) => {
        void router.navigate({ href });
      }}
    >
      <Shell pathname={pathname} config={config}>
        <Outlet />
      </Shell>
    </NavigationProvider>
  );
}

function NotFound() {
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.not_found")}</ScreenHeading>
      <FeedNotice>{t("app.not_found")}</FeedNotice>
    </>
  );
}
