// SPDX-License-Identifier: AGPL-3.0-only

// Small values a tab keeps while the app is left for another site and comes back (a login provider's page): what the
// reader had entered and would otherwise enter again, never a secret. They live in the tab alone and go with it. A
// browser that keeps nothing (storage blocked) answers null and forgets.

/** The value this tab keeps under `key`, or null when none is kept or the browser keeps nothing. */
export function readTabValue(key: string): string | null {
  try {
    return sessionStorage.getItem(key);
  } catch {
    return null;
  }
}

/** Keeps `value` under `key` in this tab, or forgets it with null; a browser that keeps nothing ignores it. */
export function writeTabValue(key: string, value: string | null): void {
  try {
    if (value === null) sessionStorage.removeItem(key);
    else sessionStorage.setItem(key, value);
  } catch {
    // The browser keeps nothing: the value goes with the page.
  }
}
