// SPDX-License-Identifier: AGPL-3.0-only
// The hashtags used lately on this browser (ADR-0092 §1): sending a post puts its hashtags first in
// lib/storage `recentTagsKey` (16 at most), and the completion after "#" lists those that start with the typed word
// before the server's suggestions. The server keeps nothing of it.
import { maxRecentTags, recentTagsKey } from "../../../lib/storage";
import { textTokens } from "../../../lib/textTokens";

/** A hashtag as the server compares it (NormalizeTag): NFKC and lower case. */
function normalizeTag(tag: string): string {
  return tag.normalize("NFKC").toLowerCase();
}

/** The list `recent` after a post with the hashtags `used`: those first, in their order, each once. */
function withRecentTags(recent: readonly string[], used: readonly string[]): string[] {
  const seen = new Set<string>();
  const list: string[] = [];
  for (const tag of [...used, ...recent]) {
    const key = normalizeTag(tag);
    if (seen.has(key)) continue;
    seen.add(key);
    list.push(tag);
  }
  return list.slice(0, maxRecentTags);
}

/** Puts the hashtags of the text of a post being sent first in the hashtags used lately. */
export function rememberTags(text: string): void {
  const used = textTokens(text).flatMap((token) => (token.kind === "hashtag" ? [token.text.slice(1)] : []));
  if (used.length === 0) return;
  recentTagsKey.write(withRecentTags(recentTagsKey.read(), used));
}

/** A hashtag the completion offers: its name, and how many posts carry it when the server knows. */
export type TagChoice = { name: string; statusesCount: number | undefined };

/**
 * The hashtags to offer for the query `query`: those of `recent` that start with it first (the latest first), then the
 * server's suggestions `found` in their order, each once, at most `limit`.
 */
export function rankTags(query: string, found: readonly { name: string; statuses_count: number }[], recent: readonly string[], limit: number): TagChoice[] {
  const prefix = normalizeTag(query);
  const counts = new Map(found.map((tag) => [normalizeTag(tag.name), tag.statuses_count]));
  const choices: TagChoice[] = [];
  const seen = new Set<string>();
  const add = (name: string) => {
    const key = normalizeTag(name);
    if (seen.has(key)) return;
    seen.add(key);
    choices.push({ name, statusesCount: counts.get(key) });
  };
  for (const tag of recent) if (normalizeTag(tag).startsWith(prefix)) add(tag);
  for (const tag of found) add(tag.name);
  return choices.slice(0, limit);
}
