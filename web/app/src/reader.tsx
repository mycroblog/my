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
// own language or zone, and it is forgotten once the app runs without a session.

import { usePreferences, type Preferences } from "@my/app-data";
import { I18nProvider, TimeZoneProvider, useLocale } from "@my/design-system";
import { readDeviceValue, writeDeviceValue } from "@my/platform";
import { useEffect, useLayoutEffect, useState, type ReactNode } from "react";
import { inCatalog } from "./locale";

/** What an account chose in place of the browser's: a locale (a BCP 47 tag), a time zone (an IANA name). */
type Choice = { readonly locale?: string | undefined; readonly timeZone?: string | undefined };

/** The device's key of the choice of the account that last read its preferences here. */
const choiceKey = "my:reader-choice";

const choiceOf = (preferences: Preferences): Choice => ({
  locale: preferences.locale_follows_browser ? undefined : preferences.locale,
  timeZone: preferences.time_zone_follows_browser ? undefined : preferences.time_zone,
});

/** The choice kept on the device; none where nothing is kept, or what is kept is no choice. */
function keptChoice(): Choice {
  let kept: unknown;
  try {
    kept = JSON.parse(readDeviceValue(choiceKey) ?? "{}");
  } catch {
    return {};
  }
  if (kept === null || typeof kept !== "object") return {};
  return {
    locale: "locale" in kept && typeof kept.locale === "string" ? kept.locale : undefined,
    timeZone: "timeZone" in kept && typeof kept.timeZone === "string" ? kept.timeZone : undefined,
  };
}

/** Keeps `choice` on the device, or forgets what is kept when it chooses nothing (or is null, without a session). */
function keepChoice(choice: Choice | null): void {
  const value =
    choice === null || (choice.locale === undefined && choice.timeZone === undefined) ? null : JSON.stringify(choice);
  if (readDeviceValue(choiceKey) !== value) writeDeviceValue(choiceKey, value);
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
 * the browser's. It is placed under the I18nProvider of the browser's locale and the AppDataProvider.
 */
export function Reader({ signedIn, children }: { signedIn: boolean; children: ReactNode }) {
  const browser = useLocale().locale;
  const preferences = usePreferences(signedIn);
  const [kept] = useState(keptChoice);
  const choice = !signedIn ? {} : preferences === undefined ? kept : choiceOf(preferences);
  useEffect(() => {
    if (!signedIn) keepChoice(null);
    else if (preferences !== undefined) keepChoice(choiceOf(preferences));
  }, [signedIn, preferences]);
  const chosen = choice.locale === undefined ? undefined : inCatalog(choice.locale);
  return (
    <I18nProvider locale={chosen ?? browser}>
      <TimeZoneProvider timeZone={choice.timeZone}>
        <Html under={browser}>{children}</Html>
      </TimeZoneProvider>
    </I18nProvider>
  );
}
