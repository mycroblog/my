// SPDX-License-Identifier: AGPL-3.0-only

// Whether a moment of the closure (its closing date) has passed, as it passes: the screen that offers to close the
// server once it has draws again then, not only at its next visit.

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
