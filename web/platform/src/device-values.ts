// SPDX-License-Identifier: AGPL-3.0-only

// Small values the app keeps on the device across its loads (ADR-0028 §18, the device's state), which are not the
// reader's data: what the app must still show when the server does not answer, such as the status page's address
// (ADR-0088 §20). A browser that keeps nothing (storage blocked, a private window) answers null and forgets.

/** The value kept under `key`, or null when none is kept or the device keeps nothing. */
export function readDeviceValue(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

/** Keeps `value` under `key`, or forgets it with null; a device that keeps nothing ignores it. */
export function writeDeviceValue(key: string, value: string | null): void {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // The device keeps nothing: the value lives until the page goes.
  }
}
