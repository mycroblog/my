// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useSyncExternalStore } from "react";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { resolveLocale } from "../../../lib/i18n";
import { shellQueryRetry } from "../../../lib/query";
import { useAppLocale } from "../../../lib/locale";
import { cacheSavedPreferences, type PreferencesChange, preferencesQueryOptions, saveDetected } from "../api/me";
import { deviceTimeZone, sameTimeZone } from "./timeZones";

/** The browser's languages in its order of preference, as one string so that it can be a snapshot. */
function browserLanguages(): string {
  return navigator.languages.join(",");
}

function subscribeToLanguages(listener: () => void): () => void {
  window.addEventListener("languagechange", listener);
  return () => {
    window.removeEventListener("languagechange", listener);
  };
}

/**
 * Displays the app in the signed-in user's locale and time zone (FR-007): at start-up, and at once when the
 * preferences change, in this tab or after a refetch. The page starts in the instance's locale, and keeps the
 * last one it showed when nobody is signed in.
 *
 * A user who follows the browser, as a new account does, sees this browser's language and time zone, and what is
 * saved stays what was last detected, for what has no browser (email, push). A user who has no time zone saved yet
 * sees and saves this device's too.
 */
export function useUserDisplaySettings(): void {
  const api = useApi();
  const queryClient = useQueryClient();
  const status = useSessionStatus();
  const appLocale = useAppLocale();
  const { data } = useQuery({
    ...preferencesQueryOptions(api),
    enabled: status === "signed-in",
    retry: shellQueryRetry,
  });
  const languages = useSyncExternalStore(subscribeToLanguages, browserLanguages);
  const browserLocale = appLocale.browserLocale(languages.split(","));
  const savedLocale = data?.locale;
  const savedZone = data?.time_zone;
  const followsLocale = data?.locale_follows_browser === true;
  const followsZone = data?.time_zone_follows_browser === true;
  const unchosen = data?.time_zone_chosen === false;
  const locale = followsLocale ? browserLocale : savedLocale;
  // Undefined is the browser's time zone: until the preferences load, while following it, and while none is saved.
  const timeZone = followsZone || unchosen ? undefined : savedZone;

  // The locale store is outside React (lib/locale): loading a catalog and setting <html lang dir>.
  useEffect(() => {
    if (locale !== undefined) void appLocale.setLocale(locale);
  }, [appLocale, locale]);
  useEffect(() => {
    appLocale.setTimeZone(timeZone);
  }, [appLocale, timeZone]);

  // Saves what this browser says (a browser API) when it differs from what is saved. It is not the user's action, so
  // it is not in an event handler. A failure leaves the saved values as they are; the next page load tries again.
  useEffect(() => {
    if (savedLocale === undefined || savedZone === undefined) return;
    const change: PreferencesChange = {};
    if (followsLocale && resolveLocale(savedLocale) !== browserLocale) {
      change.locale = browserLocale;
      change.locale_follows_browser = true;
    }
    const zone = deviceTimeZone();
    if (unchosen || (followsZone && !sameTimeZone(savedZone, zone))) {
      change.time_zone = zone;
      if (followsZone) change.time_zone_follows_browser = true;
    }
    if (change.locale === undefined && change.time_zone === undefined) return;
    const controller = new AbortController();
    saveDetected(api, change, controller.signal).then(
      (preferences) => cacheSavedPreferences(api, queryClient, preferences, change),
      () => {
        // Cancelled, or the server does not know the value: the app still shows what the browser says.
      },
    );
    return () => {
      controller.abort();
    };
  }, [api, queryClient, browserLocale, followsLocale, followsZone, savedLocale, savedZone, unchosen]);
}
