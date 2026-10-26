// SPDX-License-Identifier: AGPL-3.0-only
// The layout around every page (ADR-0026). Mobile first (web-client/README.md D1): one column with the
// navigation as a bottom tab bar; from md a side rail and the page; from xl a third column with the open voice chat
// rooms and about the server; the admin and moderation screens have a frame of their own (AdminLayout). Its look is @my/ui's AppShell; this file decides what goes in it.
import { AppShell } from "@my/ui";
import { Outlet, useLocation, useMatches } from "@tanstack/react-router";
import { useRef } from "react";
import { ErrorBoundary } from "react-error-boundary";
import { ClosedScreen, ClosureBanner } from "../../features/closure";
import { ComposeEntry, ComposeProvider } from "../../features/compose";
import { useViewer } from "../../features/session";
import { useUserDisplaySettings } from "../../features/settings";
import { VoiceCallBar, VoiceRail } from "../../features/voice";
import { hasClosed, useClosure } from "../../lib/closure";
import { useT } from "../../lib/i18n";
import { AdminLayout, isStaffMode } from "./AdminLayout";
import { ConnectionBanner } from "./ConnectionBanner";
import { Navigation, useSignedOut } from "./Navigation";
import { ServerInfo } from "./ServerInfo";
import { useLiveUpdates } from "./useLiveUpdates";
import { useMediaQuery } from "./useMediaQuery";
import { useViewChange } from "./useViewChange";

/** The id of the main landmark, the target of the skip link. */
const mainId = "main";

/** The `xl` breakpoint of Tailwind (80rem), from which the third column shows. */
const wideQuery = "(min-width: 80rem)";

/** The `md` breakpoint of Tailwind (48rem), from which the rail has the new-post button and nothing floats. */
const mediumQuery = "(min-width: 48rem)";

export function AppLayout() {
  const t = useT();
  const mainRef = useRef<HTMLElement>(null);
  useViewChange(mainRef);
  useLiveUpdates();
  // Every page is displayed in the signed-in user's locale and time zone.
  useUserDisplaySettings();
  const wide = useMediaQuery(wideQuery);
  const medium = useMediaQuery(mediumQuery);
  // Only pages with a feed float the new-post button (timelines and profiles: their routes' static data), below md
  // (ADR-0039); from md the rail has the button.
  const pageWantsButton = useMatches({ select: (matches) => matches.some((match) => match.staticData.composeButton === true) });
  const wideMain = useMatches({ select: (matches) => matches.some((match) => match.staticData.wideMain === true) });
  const signedOut = useSignedOut();
  const signedIn = useViewer().status === "signed-in";
  const closure = useClosure();
  const showButton = !medium && pageWantsButton && signedIn;
  // The admin and moderation screens are a mode of their own, with a frame and a menu of their own (ADR-0058).
  const pathname = useLocation({ select: (location) => location.pathname });
  const staffMode = isStaffMode(pathname);
  // The voice chat pages list the rooms themselves, so the third column does not repeat them.
  const onVoicePage = pathname === "/voice" || pathname.startsWith("/voice/");
  if (staffMode) {
    return (
      <ComposeProvider>
        <ConnectionBanner />
        <ClosureBanner />
        <AdminLayout skipLinkLabel={t("client:shell.skip_to_main")} mainId={mainId} mainRef={mainRef}>
          <Outlet />
          <VoiceCallBar clearsTabBar={false} />
        </AdminLayout>
      </ComposeProvider>
    );
  }
  // Once the server has closed, a signed-in user has the closed screen and nothing else of the app: the client API refuses
  // the rest (ADR-0088 §2). The staff's screens above stay, since the closure's progress is followed there.
  if (hasClosed(closure) && signedIn) {
    return (
      <>
        <ConnectionBanner />
        <ClosedScreen closure={closure} />
      </>
    );
  }
  return (
    <ComposeProvider>
      {/* The one bar for being offline or rate-limited (web-client/16-look-states-and-wording.md G17), in the page from the start so that the
          change is announced. It pads for the top safe-area inset itself, so it comes before the shell, which pads for it. */}
      <ConnectionBanner />
      {/* The notice of the server's closure, not closable, above the page on every screen while it is announced. */}
      <ClosureBanner />
      <AppShell
        navigation={<Navigation />}
        navigationPlacement={signedOut ? "top" : "side"}
        skipLinkLabel={t("client:shell.skip_to_main")}
        width={wideMain ? "wide" : "narrow"}
        mainId={mainId}
        mainRef={mainRef}
        floatingVisible={showButton}
        // Above the tab bar on phones (only its icon, named by its label). It always holds the new-post sheet, which pages
        // without a feed keep for replies; the button shows only on pages with a feed.
        floating={<ComposeEntry trigger={showButton ? "icon" : "none"} />}
        {...(wide && {
          aside: (
            // Independent sections: if one breaks, the page goes on without it (web-client/README.md E1). The open voice
            // chat rooms come first, while there are any (web-client/09-app-shell.md §9).
            <>
              {!onVoicePage && (
                <ErrorBoundary fallback={null}>
                  <VoiceRail />
                </ErrorBoundary>
              )}
              <ErrorBoundary fallback={null}>
                <ServerInfo />
              </ErrorBoundary>
            </>
          ),
        })}
      >
        <Outlet />
        {/* The call goes on across pages: its bar ends every page while it lasts (web-client/09-app-shell.md §9). */}
        <VoiceCallBar />
      </AppShell>
    </ComposeProvider>
  );
}
