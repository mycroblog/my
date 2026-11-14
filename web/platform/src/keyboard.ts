// SPDX-License-Identifier: AGPL-3.0-only

/** Whether the reader's keyboard names the shortcut modifier ⌘ (macOS, iOS, iPadOS) rather than Ctrl. */
export function namesCommandKey(): boolean {
  const browser = globalThis as unknown as { navigator?: { platform?: string } };
  return /mac|iphone|ipad|ipod/i.test(browser.navigator?.platform ?? "");
}
