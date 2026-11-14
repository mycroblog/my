// SPDX-License-Identifier: AGPL-3.0-only

// The root of every screen: the app's shell around the screen's route, with the links of the design system's parts
// navigating through the router. An address the app has no screen for shows its own heading inside the shell. Every
// screen takes the composer's search (shell/compose.tsx), which opens the composer over it.

import { FeedNotice, NavigationProvider } from "@my/design-system";
import { watchKeyboardInset } from "@my/platform";
import { createRootRouteWithContext, Outlet, useRouter, useRouterState } from "@tanstack/react-router";
import { useEffect } from "react";
import { useStrings } from "../locale";
import type { RouterContext } from "../router";
import { parseComposeSearch } from "../shell/compose";
import { ScreenHeading, useFocusOnNavigation } from "../shell/heading";
import { useScreenScroll } from "../shell/scroll";
import { Shell } from "../shell/shell";

export const Route = createRootRouteWithContext<RouterContext>()({
  validateSearch: parseComposeSearch,
  component: Root,
  notFoundComponent: NotFound,
  // An address with no screen opens at the top.
  staticData: { scroll: "top" },
});

/** Keeps `--keyboard-inset` at the height the software keyboard covers, where the platform measures it (Safari). */
function useKeyboardInset() {
  useEffect(
    () =>
      watchKeyboardInset((inset) => {
        document.documentElement.style.setProperty("--keyboard-inset", `${String(inset)}px`);
      }),
    [],
  );
}

function Root() {
  const router = useRouter();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const { config, data } = Route.useRouteContext();
  useFocusOnNavigation(pathname);
  useScreenScroll();
  useKeyboardInset();
  return (
    <NavigationProvider
      navigate={(href) => {
        void router.navigate({ href });
      }}
    >
      <Shell pathname={pathname} config={config} signedIn={data !== null}>
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
