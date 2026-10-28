// SPDX-License-Identifier: AGPL-3.0-only
// The Service Worker of the web client (ADR-0028), built by vite-plugin-pwa's injectManifest strategy
// into dist/sw.js, which `my serve` serves at /sw.js without long-term caching. It follows the plugin's
// template (https://vite-pwa-org.netlify.app/guide/inject-manifest.html) and Workbox's modules
// (https://developer.chrome.com/docs/workbox/modules):
//
// - the hashed files of the build are precached, together with the generic HTML shell at /home (the shell of /
//   carries the landing page's link preview tags, so it is not replayed for other URLs);
// - navigations go to the network, which decides the status, the link preview tags and noindex of every
//   page; when it fails or takes longer than 3 seconds, the precached shell starts the app offline. No
//   navigation response is stored, so a page's own shell is never replayed for another URL;
// - public images under /media/ are cache first, at most 300 entries for 30 days;
// - the instance theme's stylesheets under /theme/ (ADR-0059), named by their revision, are cache first, at
//   most 5 entries;
// - the instance's own icon and logos under /brand/ (ADR-0116), named by their revision, are cache first, at most 30
//   entries (the files of the current icon and logos, with room for a change), so that the offline shell shows them;
// - the Noto Color Emoji splits of the build (ADR-0095 §3), which the precache leaves out (about 6 MB in both
//   formats), are cache first when a page first shows one of their emoji, at most 44 entries (two builds of the
//   splits of both formats);
// - the client API (/api), the server-rendered pages (/auth, /oauth) and every other request are not
//   answered by the worker at all, so nothing they return is cached;
// - a Web Push message (FR-206) is shown as a notification (push.ts), and tapping it opens its page in an open
//   window of the app, or in a new one.
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
import {
  cachePrefix,
  cachesClearedMessage,
  clearCachesMessage,
  emojiFontCacheName,
  isMessage,
  mediaCacheName,
  shellPath,
  themeCacheName,
  brandCacheName,
  skipWaitingMessage,
} from "../src/lib/sw-protocol.ts";
import { clickedURL, notificationFor } from "./push.ts";
import { classify, type Handling } from "./routing.ts";

declare let self: ServiceWorkerGlobalScope & {
  /** The precache manifest that vite-plugin-pwa injects at build time. */
  __WB_MANIFEST: (PrecacheEntry | string)[];
};

/** How long a navigation may take before the offline shell is shown (ADR-0028). */
const navigationTimeoutSeconds = 3;

setCacheNameDetails({ prefix: cachePrefix });
// The first worker takes over the open pages when it activates, so the first visit already works
// offline. A later version activates only when the user applies it (SKIP_WAITING below); the page then
// reloads into it.
clientsClaim();

/**
 * A revision of the shell for the precache: it changes whenever any file of the build changes, since the
 * shell names the build's hashed entry files. FNV-1a over the manifest, which is the same in every build of
 * the same sources.
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

// Registered before the precache's route, so that a navigation to the shell's path also goes to the network first.
registerRoute(
  (options) => handlingOf(options) === "navigation",
  new NetworkOnly({
    networkTimeoutSeconds: navigationTimeoutSeconds,
    plugins: [new PrecacheFallbackPlugin({ fallbackURL: shellPath })],
  }),
);

const mediaExpiration = new ExpirationPlugin({
  maxEntries: 300,
  maxAgeSeconds: 30 * 24 * 60 * 60,
  purgeOnQuotaError: true,
});
registerRoute(
  (options) => handlingOf(options) === "media",
  new CacheFirst({
    cacheName: mediaCacheName,
    plugins: [new CacheableResponsePlugin({ statuses: [200] }), mediaExpiration],
  }),
);

const themeExpiration = new ExpirationPlugin({ maxEntries: 5, purgeOnQuotaError: true });
registerRoute(
  (options) => handlingOf(options) === "theme",
  new CacheFirst({
    cacheName: themeCacheName,
    plugins: [new CacheableResponsePlugin({ statuses: [200] }), themeExpiration],
  }),
);

const brandExpiration = new ExpirationPlugin({ maxEntries: 30, purgeOnQuotaError: true });
registerRoute(
  (options) => handlingOf(options) === "brand",
  new CacheFirst({
    cacheName: brandCacheName,
    plugins: [new CacheableResponsePlugin({ statuses: [200] }), brandExpiration],
  }),
);

const emojiFontExpiration = new ExpirationPlugin({ maxEntries: 44, purgeOnQuotaError: true });
registerRoute(
  (options) => handlingOf(options) === "emoji-font",
  new CacheFirst({
    cacheName: emojiFontCacheName,
    plugins: [new CacheableResponsePlugin({ statuses: [200] }), emojiFontExpiration],
  }),
);

// The precached build files, answered from the cache.
addRoute();

/** Deletes every cache, the precache included, and the expiration metadata of the runtime caches. */
async function clearCaches(): Promise<void> {
  await Promise.all([
    mediaExpiration.deleteCacheAndMetadata(),
    themeExpiration.deleteCacheAndMetadata(),
    emojiFontExpiration.deleteCacheAndMetadata(),
  ]);
  const names = await caches.keys();
  await Promise.all(names.map((name) => caches.delete(name)));
}

self.addEventListener("message", (event) => {
  if (isMessage(event.data, skipWaitingMessage.type)) {
    void self.skipWaiting();
  } else if (isMessage(event.data, clearCachesMessage.type)) {
    const port = event.ports[0];
    event.waitUntil(
      clearCaches().finally(() => {
        port?.postMessage(cachesClearedMessage);
      }),
    );
  }
});

// Every message is shown (the subscription is userVisibleOnly), before the event's promise settles.
self.addEventListener("push", (event) => {
  const shown = notificationFor(event.data?.text() ?? null, self.location.origin);
  event.waitUntil(self.registration.showNotification(shown.title, shown.options));
});

/**
 * Opens `url` in a window of the app: the focused one, or else any, which navigates there; a new window when the
 * app is not open (https://developer.mozilla.org/docs/Web/API/Clients/openWindow).
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
