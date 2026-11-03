// SPDX-License-Identifier: AGPL-3.0-only
// Which accounts said no to the prompt to turn on Web Push on this browser (ADR-0137 §4). Web Push belongs to a device,
// so this does too, and each account has its own answer: another user of a shared browser is still asked.
import { pushPromptDismissedKey, useStorage } from "../../../lib/storage";

/** Keeps that the account closed the prompt or turned Web Push off. */
export function rememberPushPromptDismissed(accountId: string): void {
  const dismissed = pushPromptDismissedKey.read();
  if (!dismissed.includes(accountId)) pushPromptDismissedKey.write([...dismissed, accountId]);
}

/** Forgets it when the account turns Web Push on, so that the prompt comes when signing in again stops it. */
export function forgetPushPromptDismissed(accountId: string): void {
  const dismissed = pushPromptDismissedKey.read();
  if (!dismissed.includes(accountId)) return;
  const rest = dismissed.filter((id) => id !== accountId);
  if (rest.length === 0) pushPromptDismissedKey.remove();
  else pushPromptDismissedKey.write(rest);
}

/** Whether the account said no on this browser. */
export function usePushPromptDismissed(accountId: string | undefined): boolean {
  const dismissed = useStorage(pushPromptDismissedKey);
  return accountId !== undefined && dismissed.includes(accountId);
}
