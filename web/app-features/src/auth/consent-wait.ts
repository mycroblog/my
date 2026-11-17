// SPDX-License-Identifier: AGPL-3.0-only

// The consent's wait: Allow and Deny stay disabled for a few seconds after the page opens, so that nobody allows what
// they have not read, under a caption that counts down by whole seconds. A count, not a motion, so reduced motion
// changes nothing. The form also refuses to be sent while it waits, so that no key (Enter in the page) sends it.

import { useEffect, useState } from "react";

/** How long the consent waits after the page opens, in seconds. */
export const consentWaitSeconds = 5;

/** The whole seconds left of the wait `elapsedMs` after the page opened: from `wait` down to 1, then 0. */
export function secondsLeft(elapsedMs: number, wait: number = consentWaitSeconds): number {
  return Math.max(0, Math.ceil(wait - elapsedMs / 1000));
}

/** Stops a form's submission while the wait is not over. */
export function holdWhileWaiting(left: number, event: { preventDefault(): void }): void {
  if (left > 0) event.preventDefault();
}

/** The seconds left of the consent's wait, counted from when the page shows it; 0 once it is over. */
export function useConsentWait(wait: number = consentWaitSeconds): number {
  const [left, setLeft] = useState(wait);
  useEffect(() => {
    const opened = performance.now();
    const tick = setInterval(() => {
      const next = secondsLeft(performance.now() - opened, wait);
      setLeft(next);
      if (next === 0) clearInterval(tick);
    }, 250);
    return () => {
      clearInterval(tick);
    };
  }, [wait]);
  return left;
}
