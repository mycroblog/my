// SPDX-License-Identifier: AGPL-3.0-only

// The Service Worker of the web client (ADR-0028 §3), built by vite-plugin-pwa's injectManifest into sw.js, which
// `my serve` serves at /sw.js without long-term caching (Workbox's modules: https://developer.chrome.com/docs/workbox):
//
// - the build's hashed scripts, stylesheets and images are precached, with the generic HTML shell at /home (the shell
//   of / carries the landing page's link preview, so it is not replayed for another address);
// - a navigation goes to the network, which decides every page's status, link preview and noindex; when it fails or
//   takes longer than 3 seconds, the precached shell starts the app offline. No navigation's answer is kept;
// - public images under /media/ are cache first, at most 300 for 30 days; the instance theme's stylesheets under
//   /theme/ and its icon and logos under /brand/ (ADR-0059), named by their revision, are cache first, at most 5 and
//   30; the build's font splits, too large to precache, are cache first as a page first uses them, at most 300;
// - the client API, the server's own pages and every other request are not answered by the worker at all;
// - signing out (/auth/sign_out) deletes every cache;
// - a Web Push message (FR-206) is shown as a notification (push.ts), and a click on one opens its page.
//
// A new build: its worker precaches the new files beside the old ones and waits, while the old worker keeps the
// pages open on the old build working, as their files are still precached; a navigation always fetches the new shell
// from the network, which loads the new files from the network meanwhile. The new worker takes over at once only where
// no page of the app is open, else when a page asks (Update, which then loads again: skipWaitingMessage), or once every
// page closed. Taking over, it deletes the old precache; a page left on the old build then fails to load an old chunk
// and loads again (vite:preloadError, src/main.tsx).

import { clientsClaim, setCacheNameDetails } from "workbox-core";
import { CacheableResponsePlugin } from "workbox-cacheable-response";
import { ExpirationPlugin } from "workbox-expiration";
import {
  addRoute,
  cleanupOutdatedCaches,
  precache,
  PrecacheFallbackPlugin,
  type PrecacheEntry,
} from "workbox-precaching";
import { registerRoute } from "workbox-routing";
import { CacheFirst, NetworkOnly } from "workbox-strategies";
import { cachePrefix, isMessage, shellPath, skipWaitingMessage } from "@my/platform/sw-protocol";
import { clickedURL, notificationFor } from "./push";
import { classify, signsOut, type Handling } from "./routing";

declare const self: ServiceWorkerGlobalScope & {
  /** The precache manifest that vite-plugin-pwa injects at build time. */
  __WB_MANIFEST: (PrecacheEntry | string)[];
};

/** How long a navigation may take before the offline shell is shown. */
const navigationTimeoutSeconds = 3;

setCacheNameDetails({ prefix: cachePrefix });
// The first worker takes over the open pages as it activates, so that the first visit already works offline.
clientsClaim();

/**
 * A revision of the shell for the precache, which changes whenever a file of the build changes, since the shell names
 * the build's hashed entries: FNV-1a over the manifest, the same in every build of the same sources.
 */
function shellRevision(entries: readonly (PrecacheEntry | string)[]): string {
  let hash = 0x811c9dc5;
  for (const entry of entries) {
    const text = typeof entry === "string" ? entry : `${entry.url} ${entry.revision ?? ""}`;
    for (let i = 0; i < text.length; i++) {
      hash ^= text.charCodeAt(i);
      hash = Math.imul(hash, 0x01000193) >>> 0;
    }
  }
  return hash.toString(16);
}

const manifest = self.__WB_MANIFEST;
cleanupOutdatedCaches();
precache([...manifest, { url: shellPath, revision: shellRevision(manifest) }]);

const handlingOf = ({ request }: { request: Request }): Handling =>
  classify(
    {
      url: request.url,
      method: request.method,
      mode: request.mode,
      destination: request.destination,
      authorized: request.headers.has("Authorization"),
    },
    self.location.origin,
  );

// Before the precache's route, so that a navigation to the shell's own path also goes to the network first.
registerRoute(
  (options) => handlingOf(options) === "navigation",
  new NetworkOnly({
    networkTimeoutSeconds: navigationTimeoutSeconds,
    plugins: [new PrecacheFallbackPlugin({ fallbackURL: shellPath })],
  }),
);

/** A cache-first route of `handling`, in the cache `name`, at most `maxEntries`, for `maxAgeSeconds` if given. */
function cacheFirst(handling: Handling, name: string, maxEntries: number, maxAgeSeconds?: number) {
  registerRoute(
    (options) => handlingOf(options) === handling,
    new CacheFirst({
      cacheName: `${cachePrefix}-${name}`,
      plugins: [
        new CacheableResponsePlugin({ statuses: [200] }),
        new ExpirationPlugin({
          maxEntries,
          ...(maxAgeSeconds === undefined ? {} : { maxAgeSeconds }),
          purgeOnQuotaError: true,
        }),
      ],
    }),
  );
}

cacheFirst("media", "media", 300, 30 * 24 * 60 * 60);
cacheFirst("theme", "theme", 5);
cacheFirst("brand", "brand", 30);
cacheFirst("font", "font", 300);

// The precached build files, answered from the cache.
addRoute();

/** Deletes every cache of the worker's origin, the precache included. */
async function clearCaches(): Promise<void> {
  const names = await caches.keys();
  await Promise.all(names.map((name) => caches.delete(name)));
}

// Signing out deletes every cache, as the request goes on to the network untouched (the worker does not answer it).
self.addEventListener("fetch", (event) => {
  if (signsOut(event.request.url, self.location.origin)) event.waitUntil(clearCaches());
});

// A new worker takes over at once where no page of the app is open; else when a page asks, or once they all closed.
self.addEventListener("install", (event) => {
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then(async (windows) => {
      if (windows.length === 0) await self.skipWaiting();
    }),
  );
});

self.addEventListener("message", (event) => {
  if (isMessage(event.data, skipWaitingMessage.type)) void self.skipWaiting();
});

// Every message is shown (the subscription is userVisibleOnly), before the event's promise settles.
self.addEventListener("push", (event) => {
  const shown = notificationFor(event.data?.text() ?? null, self.location.origin);
  event.waitUntil(self.registration.showNotification(shown.title, shown.options));
});

/**
 * Opens `url` in a window of the app: the focused one, or else any, which goes there; a new window when the app is not
 * open.
 */
async function openApp(url: string): Promise<void> {
  const windows = await self.clients.matchAll({ type: "window" });
  const target = windows.find((client) => client.focused) ?? windows[0];
  if (target === undefined) {
    await self.clients.openWindow(url);
    return;
  }
  const focused = await target.focus();
  await focused.navigate(url);
}

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(openApp(clickedURL(event.notification.data, self.location.origin)));
});
