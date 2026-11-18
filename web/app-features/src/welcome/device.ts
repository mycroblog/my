// SPDX-License-Identifier: AGPL-3.0-only

// What the welcome step keeps on the device (@my/platform's device values): the accounts that passed it here, by
// continuing or by skipping it. The server keeps no such fact, so an account that gave neither a name nor an avatar
// is told from a new one only on the device it passed the step on. It outlives the session, as who closed the Web
// Push prompt does; a device that keeps nothing forgets it, and asks again.

import { readDeviceValue, writeDeviceValue } from "@my/platform";

/** The device's key of the accounts that passed the welcome step here; kept on sign-out. */
const passedKey = "my:welcome-passed";

function passedAccounts(): readonly string[] {
  try {
    const parsed: unknown = JSON.parse(readDeviceValue(passedKey) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((id): id is string => typeof id === "string") : [];
  } catch {
    return [];
  }
}

/** Whether the account of `id` passed the welcome step on this device. */
export const welcomePassed = (id: string): boolean => passedAccounts().includes(id);

/** Notes that the account of `id` passed the welcome step on this device. */
export function noteWelcomePassed(id: string): void {
  const kept = passedAccounts();
  if (!kept.includes(id)) writeDeviceValue(passedKey, JSON.stringify([...kept, id]));
}
