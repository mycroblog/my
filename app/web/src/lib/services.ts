// SPDX-License-Identifier: AGPL-3.0-only
// The app's long-lived objects, made once per page: the locale the app is displayed in, the session (the
// in-memory access token), the API client on both, whether the server is under maintenance (which the session and
// the API client report to), and the QueryClient, which forgets the user's data when the user changes and refetches
// the server's localized data when the locale changes, and the reports of the page's uncaught errors. main.tsx makes them for the page and the tests make them for
// each render.
import type { QueryClient } from "@tanstack/react-query";
import { type Api, createApi } from "./api/client";
import { Session } from "./auth/session";
import type { LeaveApp } from "./auth/SessionProvider";
import { forgetUserDataOnUserChange } from "./auth/userData";
import type { RuntimeConfig } from "./config";
import { ErrorReporter } from "./errorReporting";
import { refetchInstanceOnFederationRestricted } from "./federation";
import type { I18n } from "./i18n";
import { AppLocale } from "./locale";
import { Maintenance } from "./maintenance";
import { startConnectivity } from "./network";
import { createQueryClient } from "./query";

export type AppServices = {
  queryClient: QueryClient;
  api: Api;
  session: Session;
  leaveApp: LeaveApp;
  locale: AppLocale;
  maintenance: Maintenance;
  /** The reports of the page's uncaught errors to the server (lib/errorReporting). */
  errors: ErrorReporter;
};

/** Loads `url` in place of the app. */
function assignLocation(url: URL): void {
  window.location.assign(url);
}

export type PageNavigation = {
  /** Leaves the app for a URL outside it; `location.assign` by default. */
  leaveApp?: LeaveApp | undefined;
  /** Loads the page again (the end of a maintenance); `location.reload` by default. */
  reload?: (() => void) | undefined;
};

/**
 * Makes the services for a page displayed in `i18n`, whose <html lang dir> follows the locale; tests pass
 * their own `leaveApp` and `reload`, as jsdom cannot load another document.
 */
export function createAppServices(
  config: RuntimeConfig,
  i18n: I18n,
  { leaveApp = assignLocation, reload }: PageNavigation = {},
): AppServices {
  // Each app starts from what the browser says about the network (tests make one app per test).
  startConnectivity();
  const queryClient = createQueryClient();
  const locale = new AppLocale(i18n, { doc: document, remember: true });
  const maintenance = new Maintenance({ reload });
  const session = new Session({ apiOrigin: config.apiOrigin, maintenance });
  forgetUserDataOnUserChange(session, queryClient);
  refetchOnLocaleChange(locale, queryClient);
  const api = createApi({ apiOrigin: config.apiOrigin, session, locale, maintenance });
  // The mode can narrow while a page is open; a refusal for it brings the app's offer up to date.
  refetchInstanceOnFederationRestricted(queryClient, api);
  const errors = new ErrorReporter({ api, apiOrigin: config.apiOrigin });
  return { queryClient, api, session, leaveApp, locale, maintenance, errors };
}

/** Refetches every query when the locale changes: the server localizes its texts from Accept-Language. */
function refetchOnLocaleChange(locale: AppLocale, queryClient: QueryClient): void {
  let current = locale.locale;
  locale.subscribe(() => {
    if (locale.locale === current) return;
    current = locale.locale;
    void queryClient.invalidateQueries();
  });
}
