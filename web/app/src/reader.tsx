// SPDX-License-Identifier: AGPL-3.0-only

// The language and the time zone the app shows its reader in (FR-007, ADR-0009 §2, §3), decided in one place. A
// signed-in reader's are their account's choices, the `locale` and the `time_zone` of their preferences, each unless
// the account follows the browser; a language no catalog has follows the browser too, and so does a zone the platform
// does not know. Without a session both follow the browser. The browser's locale is the one of the I18nProvider the
// app is placed under, which the entry resolves (locale.ts); its zone is the platform's. Everything under the Reader
// reads the locale from the I18nProvider it places and the zone from its TimeZoneProvider, so a choice the reader
// changes shows at once, without a reload; the <html> carries the locale and its direction.
//
// Until the preferences are read, the app shows what the account had chosen when this device last read them, kept on
// the device (ADR-0028 §18): the first paint is in the account's language, not in the browser's and then the
// account's. A device that kept nothing starts as the browser says. What is kept is the choice, never the browser's
// own language or zone, with the account it is of. Nothing tells the app whose session it opened before the first
// paint (the access token says nothing of its account), so the kept choice shows until the account is read, and no
// longer when it is another's. It is forgotten when the server says the browser has no session (not when the server
// could not be asked: a failed start keeps it for the next good one), when the reader signs out, and when a sign-in
// through the app starts a new session (routes/login.callback.tsx), so that it does not outlive its session.
//
// While the account follows the browser, the app keeps the account's `locale` and `time_zone` at what this browser
// says, for what has no browser (the server's emails and pushes; the specification's Preferences): once the server
// says the account follows the browser and holds another, the difference is sent, once and quietly. It reads what the
// server last said, not a change of the reader's that is still on its way and may fail.

import {
  usePreferences,
  useSavedPreferences,
  useUpdatePreferences,
  useViewer,
  type Preferences,
  type PreferencesChange,
} from "@my/app-data";
import { I18nProvider, TimeZoneProvider, timeZoneName, useLocale } from "@my/design-system";
import { readDeviceValue, writeDeviceValue } from "@my/platform";
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { inCatalog } from "./locale";

/** What an account chose in place of the browser's: a locale (a BCP 47 tag), a time zone (an IANA name). */
type Choice = { readonly locale?: string | undefined; readonly timeZone?: string | undefined };

/** A choice kept on the device, and the id of the account it is of. */
type Kept = Choice & { readonly account: string };

/** The device's key of the choice of the account that last read its preferences here. */
const choiceKey = "my:reader-choice";

const choiceOf = (preferences: Preferences): Choice => ({
  locale: preferences.locale_follows_browser ? undefined : preferences.locale,
  timeZone: preferences.time_zone_follows_browser ? undefined : preferences.time_zone,
});

/** The choice kept on the device; none where nothing is kept, or what is kept is no choice of an account. */
function keptChoice(): Kept | undefined {
  let kept: unknown;
  try {
    kept = JSON.parse(readDeviceValue(choiceKey) ?? "null");
  } catch {
    return undefined;
  }
  if (kept === null || typeof kept !== "object") return undefined;
  if (!("account" in kept) || typeof kept.account !== "string") return undefined;
  return {
    account: kept.account,
    locale: "locale" in kept && typeof kept.locale === "string" ? kept.locale : undefined,
    timeZone: "timeZone" in kept && typeof kept.timeZone === "string" ? kept.timeZone : undefined,
  };
}

/** Keeps `choice` on the device, or forgets what is kept when it chooses nothing (or is null). */
function keepChoice(choice: Kept | null): void {
  const value =
    choice === null || (choice.locale === undefined && choice.timeZone === undefined) ? null : JSON.stringify(choice);
  if (readDeviceValue(choiceKey) !== value) writeDeviceValue(choiceKey, value);
}

/** Forgets the choice kept on the device: the reader signs out, or a new session starts. */
export const forgetChoice = (): void => {
  keepChoice(null);
};

/**
 * Sends the account the language (`browser`) and the time zone of this browser, where its preferences as the server
 * holds them (`saved`) follow the browser and hold another: once for a difference, with no word to the reader, and
 * not again on this page when the server does not take it. Nothing without preferences (not read yet, or no session).
 */
function useBrowserKept(saved: Preferences | undefined, browser: string) {
  const update = useUpdatePreferences();
  const sent = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (saved === undefined) return;
    const zone = timeZoneName(undefined);
    const change: PreferencesChange = {
      ...(saved.locale_follows_browser && saved.locale !== browser
        ? { locale: browser, locale_follows_browser: true }
        : {}),
      ...(saved.time_zone_follows_browser && saved.time_zone !== zone
        ? { time_zone: zone, time_zone_follows_browser: true }
        : {}),
    };
    const key = JSON.stringify(change);
    if (key === "{}" || sent.current === key) return;
    sent.current = key;
    update(change).catch(() => undefined);
  }, [saved, browser, update]);
}

/**
 * The page's <html> in the locale and the direction of the I18nProvider above, before the paint; written again when
 * the locale the app is placed under (`under`) changes, since what places the app may write its own there (the
 * toolbar of the stories).
 */
function Html({ under, children }: { under: string; children: ReactNode }) {
  const { locale, direction } = useLocale();
  useLayoutEffect(() => {
    document.documentElement.lang = locale;
    document.documentElement.dir = direction;
  }, [locale, direction, under]);
  return children;
}

/**
 * Shows `children` in the reader's language and time zone: with a session (`signedIn`), the account's choices, else
 * the browser's. `unasked`: there is no session because the server could not say whether there is one, so what the
 * device keeps of the reader stays. It is placed under the I18nProvider of the browser's locale and the
 * AppDataProvider.
 */
export function Reader({
  signedIn,
  unasked = false,
  children,
}: {
  signedIn: boolean;
  unasked?: boolean;
  children: ReactNode;
}) {
  const browser = useLocale().locale;
  const preferences = usePreferences(signedIn);
  const saved = useSavedPreferences(signedIn);
  const account = useViewer(signedIn)?.id;
  const [kept] = useState(keptChoice);
  // The kept choice stands in until the preferences are read, unless the account read meanwhile is another's.
  const standIn: Choice = kept !== undefined && (account === undefined || account === kept.account) ? kept : {};
  const choice = !signedIn ? {} : preferences === undefined ? standIn : choiceOf(preferences);
  useEffect(() => {
    if (!signedIn) {
      if (!unasked) keepChoice(null);
    } else if (preferences !== undefined && account !== undefined) keepChoice({ account, ...choiceOf(preferences) });
    else if (account !== undefined && kept !== undefined && kept.account !== account) keepChoice(null);
  }, [signedIn, unasked, preferences, account, kept]);
  useBrowserKept(saved, browser);
  const chosen = choice.locale === undefined ? undefined : inCatalog(choice.locale);
  return (
    <I18nProvider locale={chosen ?? browser}>
      <TimeZoneProvider timeZone={choice.timeZone}>
        <Html under={browser}>{children}</Html>
      </TimeZoneProvider>
    </I18nProvider>
  );
}
