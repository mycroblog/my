// SPDX-License-Identifier: AGPL-3.0-only

// The root of every screen: the app's shell around the screen's route, with the links of the design system's parts
// navigating through the router (but to the server's own pages, with a page load), unless the server's maintenance or
// its closure takes the place of every screen (ADR-0088 §2, §10). A visitor at / gets the landing page in place of the
// shell. An address the app has no screen for shows its own heading inside the shell.

import { useInstance, useServerState, useWritesInFlight } from "@my/app-data";
import { FeedNotice, NavigationProvider } from "@my/design-system";
import { watchKeyboardInset } from "@my/platform";
import { createRootRouteWithContext, Outlet, useRouter, useRouterState } from "@tanstack/react-router";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useStrings } from "../locale";
import type { RouterContext } from "../router";
import { ScreenHeading, useFocusOnNavigation } from "../shell/heading";
import { Landing } from "../shell/landing";
import { ClosedPage, MaintenancePage, pageServer } from "../shell/pages";
import { useScreenScroll } from "../shell/scroll";
import { Shell } from "../shell/shell";
import { isServerPage } from "../sign-in";

export const Route = createRootRouteWithContext<RouterContext>()({
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

/** Whether the reader is typing: the focus is on a field or an editable element. */
const typing = () => {
  const active = document.activeElement;
  return active instanceof HTMLElement && (active.isContentEditable || active.matches("input, textarea, select"));
};

/**
 * Loads the app again once the maintenance it showed has ended (ADR-0088 §10), so that the session and every screen
 * start anew from the server; not while a write is on its way or the reader is typing (ADR-0028 §13), but once they
 * stop. The maintenance page has taken the place of the shell and its composer meanwhile, so no screen's text is open
 * then; the page itself holds no field.
 */
function useReloadAfterMaintenance(underMaintenance: boolean) {
  const writing = useWritesInFlight();
  const was = useRef(underMaintenance);
  const [owed, setOwed] = useState(false);
  useEffect(() => {
    if (was.current && !underMaintenance) setOwed(true);
    was.current = underMaintenance;
  }, [underMaintenance]);
  useEffect(() => {
    if (!owed || writing) return;
    const reloadWhenIdle = () => {
      if (!typing()) location.reload();
    };
    reloadWhenIdle();
    document.addEventListener("focusout", reloadWhenIdle);
    return () => {
      document.removeEventListener("focusout", reloadWhenIdle);
    };
  }, [owed, writing]);
}

/**
 * The page that takes the place of every screen, if one does: the maintenance while the server answers with it, and
 * for a signed-in reader the closed server.
 */
function ServerPage({ signedIn, children }: { signedIn: boolean; children: ReactNode }) {
  const { config } = Route.useRouteContext();
  const instance = useInstance();
  const { maintenance } = useServerState();
  useReloadAfterMaintenance(maintenance !== null);
  if (maintenance !== null) return <MaintenancePage maintenance={maintenance} />;
  const closure = instance?.closure;
  if (signedIn && closure !== undefined && (closure.state === "closed" || closure.state === "delivery_completed"))
    return <ClosedPage server={pageServer(instance, config.instanceName)} closure={closure} />;
  return children;
}

/**
 * The screen at `pathname` in the app's shell; for a visitor at /, the landing page in its place, unless the server is
 * one person's, whose operator's profile is its first page while it can be shown (ADR-0152 §7).
 */
function Screens({ pathname, signedIn, serverName }: { pathname: string; signedIn: boolean; serverName: string }) {
  const operator = useInstance()?.single_user_account;
  if (!signedIn && pathname === "/" && operator === undefined) return <Landing serverName={serverName} />;
  return (
    <Shell pathname={pathname} signedIn={signedIn} serverName={serverName}>
      <Outlet />
    </Shell>
  );
}

function Root() {
  const router = useRouter();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const { data, config } = Route.useRouteContext();
  const signedIn = data !== null;
  useFocusOnNavigation();
  useScreenScroll();
  useKeyboardInset();
  return (
    <NavigationProvider
      navigate={(href) => {
        if (isServerPage(href)) location.assign(href);
        else void router.navigate({ href });
      }}
    >
      <ServerPage signedIn={signedIn}>
        <Screens pathname={pathname} signedIn={signedIn} serverName={config.instanceName} />
      </ServerPage>
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
