// SPDX-License-Identifier: AGPL-3.0-only

/**
 * A hashtag for a sentence: `#` and its name, isolated from the text around it, as the name is the user's text in any
 * script (web-client/README.md Y10). U+2068 FIRST STRONG ISOLATE … U+2069 POP DIRECTIONAL ISOLATE.
 */
export function hashtagInText(name: string): string {
  return `⁨#${name}⁩`;
}
