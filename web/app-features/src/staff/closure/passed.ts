// SPDX-License-Identifier: AGPL-3.0-only

// Whether a moment of the closure (its closing date) has passed, as it passes: the screen that offers to close the
// server once it has draws again then, not only at its next visit; and how the closure's screens write a moment.

import { dateFormat, useLocale, useTimeZone } from "@my/design-system";
import { useEffect, useState } from "react";

/** The longest a timer waits (about 24.8 days); a later moment is checked again after it. */
const longestWait = 2 ** 31 - 1;

/** Whether `iso` has passed, true from the moment it does; false for none. */
export function usePassed(iso: string | undefined): boolean {
  const at = iso === undefined ? Number.NaN : Date.parse(iso);
  const [now, setNow] = useState(() => Date.now());
  const passed = !Number.isNaN(at) && at <= now;
  useEffect(() => {
    if (Number.isNaN(at) || passed) return;
    const timer = setTimeout(
      () => {
        setNow(Date.now());
      },
      Math.min(longestWait, Math.max(0, at - Date.now()) + 1),
    );
    return () => {
      clearTimeout(timer);
    };
  }, [at, passed, now]);
  return passed;
}

/**
 * Writes a moment of the closure as its banner writes the date, long, with its time, in the reader's zone, never
 * broken inside (no-break spaces).
 */
export function useClosureMoment(): (iso: string) => string {
  const { locale } = useLocale();
  const zone = useTimeZone();
  const format = dateFormat(locale, zone, { dateStyle: "long", timeStyle: "short" });
  return (iso) => format.format(new Date(iso)).replace(/ /g, "\u00a0");
}
