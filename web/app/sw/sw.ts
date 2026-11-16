// SPDX-License-Identifier: AGPL-3.0-only

// The Service Worker of the web client (ADR-0028 §3), which `my serve` serves at /sw.js: for now it only shows the Web
// Push messages (FR-206, push.ts) and opens what a click on one tells of, in an open window of the app, or in a new
// one. It answers no request (no fetch handler), so it caches nothing: every page and file comes from the network as
// without it; the precache, the offline shell and the runtime caches of ADR-0028 §3 come with #3147. A new version
// takes over at once, which holds nothing to keep consistent.

import { clickedURL, notificationFor } from "./push";

declare const self: ServiceWorkerGlobalScope;

self.addEventListener("install", () => {
  void self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
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
