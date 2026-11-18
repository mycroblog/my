// SPDX-License-Identifier: AGPL-3.0-only

// The caches of the page's origin, which the app's Service Worker fills (ADR-0028 §3): the build's files, the public
// images, the instance's theme and brand, the fonts. Signing out deletes every one of them, from the page, once the
// server ended the session; the worker fills them again as the next page asks.

/** Deletes every cache of the page's origin; nothing where the browser has no caches. */
export async function clearCaches(): Promise<void> {
  if (typeof caches === "undefined") return;
  const names = await caches.keys();
  await Promise.all(names.map((name) => caches.delete(name)));
}
