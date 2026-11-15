// SPDX-License-Identifier: AGPL-3.0-only

// The hashtags the writer posted lately on this device (ADR-0092 §1), which the suggestions for `#` put first among
// those the server found. They are how this device is used, like the emoji used often (ADR-0091), so they are the
// device's (@my/platform's device values), not the server's; a device that keeps nothing forgets them with the page.

import { readDeviceValue, writeDeviceValue } from "@my/platform";
import { textTokens } from "./text-tokens";

const recentKey = "my:compose:recent-tags";

/** How many hashtags are kept. */
const maxRecent = 16;

/** The hashtags posted lately, without their `#`, the latest first. */
export function recentTags(): readonly string[] {
  try {
    const parsed: unknown = JSON.parse(readDeviceValue(recentKey) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((t): t is string => typeof t === "string") : [];
  } catch {
    return [];
  }
}

/** Keeps the hashtags of a posted `text` as the latest, each once whatever its case. */
export function rememberTags(text: string): void {
  const posted = textTokens(text).flatMap((t) => (t.kind === "hashtag" ? [t.text.slice(1)] : []));
  if (posted.length === 0) return;
  const seen = new Set<string>();
  const kept = [...posted, ...recentTags()].filter((tag) => {
    const key = tag.toLowerCase();
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
  writeDeviceValue(recentKey, JSON.stringify(kept.slice(0, maxRecent)));
}
