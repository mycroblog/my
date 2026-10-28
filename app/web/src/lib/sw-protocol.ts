// SPDX-License-Identifier: AGPL-3.0-only
// What the page and the Service Worker (app/web/sw/sw.ts) share: the messages between them and the
// names of the worker's caches (ADR-0028). Both programs compile this file, so it uses neither DOM nor
// WebWorker types.

/** The prefix of every Cache Storage cache the worker creates (workbox-core's setCacheNameDetails). */
export const cachePrefix = "my";

/** The cache of public media (/media/ images), first-hit and bounded. */
export const mediaCacheName = `${cachePrefix}-media`;
/** The runtime cache of the instance theme's stylesheets (ADR-0059). */
export const themeCacheName = `${cachePrefix}-theme`;
/** The runtime cache of the instance's own icon and logos (/brand/, ADR-0116). */
export const brandCacheName = `${cachePrefix}-brand`;
/** The runtime cache of the Noto Color Emoji splits (ADR-0095 §3), which the precache leaves out. */
export const emojiFontCacheName = `${cachePrefix}-emoji-font`;

/**
 * The path of the generic HTML shell the worker precaches and returns for navigations it cannot fetch: a page
 * with no link preview of its own (the shell of / previews the instance, ADR-0028).
 */
export const shellPath = "/home";

/** Activates a waiting worker; workbox-window's messageSkipWaiting() sends it. */
export const skipWaitingMessage = { type: "SKIP_WAITING" } as const;

/** Asks the worker to delete every cache, with the media cache's metadata; it replies on the port. */
export const clearCachesMessage = { type: "CLEAR_CACHES" } as const;

/** The worker's reply to clearCachesMessage. */
export const cachesClearedMessage = { type: "CACHES_CLEARED" } as const;

/** Whether `data` is a message of type `type`. */
export function isMessage(data: unknown, type: string): boolean {
  return typeof data === "object" && data !== null && "type" in data && data.type === type;
}
