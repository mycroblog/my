// SPDX-License-Identifier: AGPL-3.0-only
// Lookups by a value the server sends. The exhaustive `Record` keeps a known value from going without its entry
// at the type check, but a server newer than this bundle can send a value the bundle does not know (ADR-0090 §8).

/** The entry of `key` in `table`, undefined for a value this bundle does not know. */
export function known<K extends string, V>(table: Readonly<Record<K, V>>, key: K): V | undefined {
  return Object.hasOwn(table, key) ? table[key] : undefined;
}
