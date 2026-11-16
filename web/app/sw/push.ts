// SPDX-License-Identifier: AGPL-3.0-only

// What the Service Worker shows of a Web Push message (FR-206) and where a click on it leads. The server encrypts one
// document per notification (internal/app/notification PushPayload): its ID, its kind, its title in the reader's
// language, and a page. The notification shows the title, tagged with the ID so that a message sent twice replaces
// itself, and keeps the page this client opens for it (pushTargetPath), on the worker's own origin only. A message the
// worker cannot read still shows, as the server's host, and leads to the notifications: a push must show something.

import { pushTargetPath } from "@my/app-components/paths";

/** A notification to show: its title and its options. */
export type Shown = { title: string; options: NotificationOptions & { data: { url: string } } };

type Payload = { notification_id: string; type: string; title: string; lang: string; path: string };

const isPayload = (value: unknown): value is Payload => {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return ["notification_id", "type", "title", "lang", "path"].every((k) => typeof v[k] === "string");
};

/** The address of `path` on `origin`; the notifications for a path that leads off it. */
function targetURL(path: string, origin: string): string {
  const fallback = new URL("/notifications", origin).href;
  if (!path.startsWith("/") || path.startsWith("//") || path.startsWith("/\\")) return fallback;
  const url = new URL(path, origin);
  return url.origin === origin ? url.href : fallback;
}

/** What to show of the message `text` (null when it carries nothing), on the worker's `origin`. */
export function notificationFor(text: string | null, origin: string): Shown {
  let payload: unknown;
  try {
    payload = text === null ? null : JSON.parse(text);
  } catch {
    payload = null;
  }
  if (!isPayload(payload))
    return { title: new URL(origin).host, options: { data: { url: targetURL("/notifications", origin) } } };
  return {
    title: payload.title,
    options: {
      tag: payload.notification_id,
      lang: payload.lang,
      dir: "auto",
      data: { url: targetURL(pushTargetPath(payload), origin) },
    },
  };
}

/** The address a click on a notification opens: the one its data keeps, on the worker's `origin` only. */
export function clickedURL(data: unknown, origin: string): string {
  const url =
    typeof data === "object" && data !== null && typeof (data as { url?: unknown }).url === "string"
      ? (data as { url: string }).url
      : "";
  try {
    if (new URL(url).origin === origin) return url;
  } catch {
    // Not an address: the notifications.
  }
  return new URL("/notifications", origin).href;
}
