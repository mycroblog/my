// SPDX-License-Identifier: AGPL-3.0-only

// An event's description as its page shows it (ADR-0161 §5, §6): plain text, where only an `http` or `https` URL is a
// link, as the server takes a URL of a place; what ends a sentence after a URL (a stop, a comma, a closing bracket) is
// text, not the link's.

import type { DescriptionPart } from "@my/design-system";

const url = /https?:\/\/[^\s<>"]+/giu;

/** What closes a URL's sentence rather than the URL: trailing punctuation, and a closing bracket it never opened. */
function trimmed(found: string): string {
  let end = found.length;
  while (end > 0) {
    const last = found[end - 1] ?? "";
    if (".,;:!?'".includes(last)) end -= 1;
    else if (last === ")" && !found.slice(0, end - 1).includes("(")) end -= 1;
    else break;
  }
  return found.slice(0, end);
}

/** The host a URL leads to, or null for a string that is no URL the page would link. */
function hostOf(href: string): string | null {
  try {
    const parsed = new URL(href);
    return (parsed.protocol === "http:" || parsed.protocol === "https:") && parsed.host !== "" ? parsed.host : null;
  } catch {
    return null;
  }
}

/** `text` as the parts of a description: its text, and each URL in it a link to its host. */
export function descriptionParts(text: string): DescriptionPart[] {
  const parts: DescriptionPart[] = [];
  let from = 0;
  for (const match of text.matchAll(url)) {
    const href = trimmed(match[0]);
    const host = hostOf(href);
    if (host === null) continue;
    if (match.index > from) parts.push({ text: text.slice(from, match.index) });
    parts.push({ link: { href, text: href, host } });
    from = match.index + href.length;
  }
  if (from < text.length) parts.push({ text: text.slice(from) });
  return parts;
}
