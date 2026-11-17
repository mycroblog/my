// SPDX-License-Identifier: AGPL-3.0-only

// What the page and the app's Service Worker (web/app/sw) share (ADR-0028 §3): the names of the worker's caches, the
// shell it falls back to, and the messages between them. Both programs compile this file, so it uses neither the
// page's nor the worker's types.

/** The prefix of every cache the worker makes (workbox-core's setCacheNameDetails). */
export const cachePrefix = "my";

/**
 * The generic HTML shell the worker precaches and answers a navigation with when the network does not: a page with no
 * link preview of its own (the shell of / previews the instance).
 */
export const shellPath = "/home";

/** Asks a waiting worker to take over (Update, which then loads the page again). */
export const skipWaitingMessage = { type: "SKIP_WAITING" } as const;

/** Whether `data` is a message of type `type`. */
export function isMessage(data: unknown, type: string): boolean {
  return typeof data === "object" && data !== null && "type" in data && data.type === type;
}
