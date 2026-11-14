// SPDX-License-Identifier: AGPL-3.0-only

// The reader's languages as their browser states them, the ones it sends as Accept-Language, from which the app
// resolves its locale (ADR-0009 §2).

/** The browser's languages, in the reader's order; none where the platform states none. */
export function browserLanguages(): readonly string[] {
  return typeof navigator === "undefined" ? [] : navigator.languages;
}
