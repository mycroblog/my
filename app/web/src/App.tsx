// SPDX-License-Identifier: AGPL-3.0-only
// The app's providers, outermost first: the runtime config, the locale store and i18n in its current locale
// (react-intl and React Aria's locale), the TanStack Query cache, the session and the API client, the live
// regions and toasts of @my/ui (with the prompt for a new version of the app), the user's voice chat call (which goes
// on across pages, features/voice), whether animations play by
// themselves (lib/autoplay), how links leave the app (ExternalLinks), React Aria's router binding, and the router
// with every page, or the maintenance of the server in its place (Maintenance.tsx).
import { AnnouncerProvider, AutoplayContext, Box, Center, ExternalLinkContext, Heading, I18nProvider as AriaI18nProvider, RouterProvider as AriaRouterProvider, Stack, Text, ToastProvider } from "@my/ui";
import { QueryClientProvider } from "@tanstack/react-query";
import { type AnyRouter, RouterProvider } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { VoiceCallProvider } from "./features/voice";
import { SessionProvider } from "./lib/auth/SessionProvider";
import { useAutoplay } from "./lib/autoplay";
import { ConfigProvider, type RuntimeConfig, useConfig } from "./lib/config";
import { contentLinkPath } from "./lib/contentLinks";
import { type I18n, MessagesProvider, useT } from "./lib/i18n";
import { AppLocaleProvider, useDisplaySettings } from "./lib/locale";
import { isServerPath } from "./lib/paths";
import type { AppServices } from "./lib/services";
import { MaintenanceGate } from "./Maintenance";
import { UpdatePrompt } from "./UpdatePrompt";

export type AppProps = AppServices & {
  config: RuntimeConfig;
  /** The app router (createAppRouter); tests may pass a router over routes of their own. */
  router: AnyRouter;
};

export function App({ config, locale, queryClient, api, session, leaveApp, maintenance, router }: AppProps) {
  const autoplay = useAutoplay();
  /**
   * Every @my/ui Link (and Menu item, Tab) with a same-origin href navigates through the router instead
   * of loading a new document. An in-page target (the skip link's "#main") gets focus instead, as the
   * browser would do for a fragment, without touching the router's location. A page of the server itself
   * (the terms of service under /auth, lib/paths.ts) is no route of the app: it loads in place of the app.
   */
  const navigate = (href: string) => {
    if (href.startsWith("#")) {
      document.getElementById(decodeURIComponent(href.slice(1)))?.focus();
      return;
    }
    const url = new URL(href, window.location.href);
    if (isServerPath(url.pathname)) {
      leaveApp(url);
      return;
    }
    void router.navigate({ href });
  };
  return (
    <ConfigProvider config={config}>
      <AppLocaleProvider locale={locale}>
        <DisplayedI18nProvider>
          <QueryClientProvider client={queryClient}>
            <SessionProvider session={session} api={api} leaveApp={leaveApp}>
              <AnnouncerProvider>
                <Toasts>
                  <UpdatePrompt />
                  <VoiceCallProvider>
                    <AutoplayContext value={autoplay}>
                      <ExternalLinks>
                        <AriaRouterProvider navigate={navigate}>
                          <MaintenanceGate maintenance={maintenance}>
                            <RouterProvider router={router} />
                          </MaintenanceGate>
                        </AriaRouterProvider>
                      </ExternalLinks>
                    </AutoplayContext>
                  </VoiceCallProvider>
                </Toasts>
              </AnnouncerProvider>
            </SessionProvider>
          </QueryClientProvider>
        </DisplayedI18nProvider>
      </AppLocaleProvider>
    </ConfigProvider>
  );
}

/** The i18n providers in the locale and time zone the app is displayed in now (lib/locale). */
function DisplayedI18nProvider({ children }: { children: ReactNode }) {
  const { i18n, timeZone } = useDisplaySettings();
  return (
    <AppI18nProvider i18n={i18n} timeZone={timeZone}>
      {children}
    </AppI18nProvider>
  );
}

export type AppI18nProviderProps = { i18n: I18n; timeZone?: string | undefined; children: ReactNode };

/** react-intl's messages and React Aria's locale (for the @my/ui components), from the same locale. */
export function AppI18nProvider({ i18n, timeZone, children }: AppI18nProviderProps) {
  return (
    <MessagesProvider i18n={i18n} timeZone={timeZone}>
      <AriaI18nProvider locale={i18n.locale}>{children}</AriaI18nProvider>
    </MessagesProvider>
  );
}

/**
 * How the links of @my/ui leave the app (web-client/README.md Y12): every ExternalLink, and every link of a post body or a
 * profile (PostContent) but those to the app's own pages (lib/contentLinks.ts), opens in a new tab and reads
 * "(opens in a new tab)" to assistive technology.
 */
export function ExternalLinks({ children }: { children: ReactNode }) {
  const t = useT();
  const { apiOrigin } = useConfig();
  return (
    <ExternalLinkContext
      value={{
        newTabLabel: t("client:shell.link.new_tab"),
        isAppLink: (url) => contentLinkPath(url.href, apiOrigin) !== null,
      }}
    >
      {children}
    </ExternalLinkContext>
  );
}

function Toasts({ children }: { children: ReactNode }) {
  const t = useT();
  return (
    <ToastProvider
      regionLabel={t("client:shell.toast.region")}
      dismissLabel={t("client:shell.toast.dismiss")}
      clearsTabBar
    >
      {children}
    </ToastProvider>
  );
}

export type StartupErrorProps = { i18n: I18n };

/** Shown instead of the app when the runtime config is missing or invalid; nothing else can work then. */
export function StartupError({ i18n }: StartupErrorProps) {
  return (
    <AppI18nProvider i18n={i18n}>
      <StartupErrorMessage />
    </AppI18nProvider>
  );
}

function StartupErrorMessage() {
  const t = useT();
  return (
    <Box as="main" paddingX="4">
      <Center>
        <Stack gap="4" width="prose">
          <Heading level={1}>{t("client:shell.startup_error.title")}</Heading>
          <Text as="p">{t("client:shell.startup_error.body")}</Text>
        </Stack>
      </Center>
    </Box>
  );
}
