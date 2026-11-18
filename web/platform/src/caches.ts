// SPDX-License-Identifier: AGPL-3.0-only

// The caches of the page's origin, which the app's Service Worker fills (ADR-0028 §3): the build's files and the shell
// it answers with offline (its precache), the public images, the instance's theme and brand, the fonts. Signing out
// deletes every one of them, from the page, once the server ended the session. The worker that filled them goes with
// them: it fills its precache only as it installs, so a worker left in place would answer an offline or slow
// navigation with nothing. The next load of the app registers the worker anew, which installs and precaches again.

/**
 * Deletes every cache of the page's origin and unregisters its Service Workers, so that the next load installs the
 * worker and its precache anew; nothing where the browser has neither.
 */
export async function clearCaches(): Promise<void> {
  if (typeof caches !== "undefined") {
    const names = await caches.keys();
    await Promise.all(names.map((name) => caches.delete(name)));
  }
  if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return;
  const registrations = await navigator.serviceWorker.getRegistrations();
  await Promise.all(registrations.map((registration) => registration.unregister()));
}
