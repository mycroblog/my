// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { shellQueryRetry } from "../../../lib/query";
import { useAppLocale } from "../../../lib/locale";
import { adoptTimeZone, cacheSavedPreferences, preferencesQueryOptions } from "../api/me";
import { deviceTimeZone } from "./timeZones";

/**
 * Displays the app in the signed-in user's locale and time zone (FR-007): at start-up, and at once when the
 * preferences change, in this tab or after a refetch. The page starts in the instance's locale, and keeps the
 * last one it showed when nobody is signed in.
 *
 * A user who has chosen no time zone yet (a new account) sees dates in this device's time zone, and it becomes
 * their choice: the first page they open after signing up saves it.
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
  const locale = data?.locale;
  const unchosen = data?.time_zone_chosen === false;
  // Undefined is the browser's time zone: until the preferences load, and while none is chosen.
  const timeZone = unchosen ? undefined : data?.time_zone;

  // The locale store is outside React (lib/locale): loading a catalog and setting <html lang dir>.
  useEffect(() => {
    if (locale !== undefined) void appLocale.setLocale(locale);
  }, [appLocale, locale]);
  useEffect(() => {
    appLocale.setTimeZone(timeZone);
  }, [appLocale, timeZone]);

  // Saves this device's time zone (a browser API) as the choice of a user who has none. It is not the user's
  // action, so it is not in an event handler. A failure leaves the zone unchosen; the next page load tries again.
  useEffect(() => {
    if (!unchosen) return;
    const controller = new AbortController();
    const zone = deviceTimeZone();
    adoptTimeZone(api, zone, controller.signal).then(
      (preferences) => cacheSavedPreferences(api, queryClient, preferences, { time_zone: zone }),
      () => {
        // Cancelled, or the server does not know the zone: dates stay in the device's time zone.
      },
    );
    return () => {
      controller.abort();
    };
  }, [api, queryClient, unchosen]);
}
