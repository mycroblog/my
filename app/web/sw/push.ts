// SPDX-License-Identifier: AGPL-3.0-only
// What the Service Worker shows of a Web Push message (FR-206). The server encrypts one JSON document per
// notification (internal/app/notification PushPayload) with its title already in the recipient's language, so
// the worker needs no catalog: it shows the title with the notification's ID as the tag, which makes a message
// that the server's job sent twice replace the first, and opens the message's page of the app when tapped.

/** The document that a push message carries. */
type PushPayload = {
  notification_id: string;
  type: string;
  title: string;
  lang: string;
  path: string;
};

/** What showNotification() takes for one message, and the page that tapping it opens (in `data`). */
export type ShownNotification = { title: string; options: NotificationOptions & { data: { url: string } } };

function isPayload(value: unknown): value is PushPayload {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return ["notification_id", "type", "title", "lang", "path"].every((key) => typeof v[key] === "string");
}

/**
 * The page of the app that a payload's path names, on `origin`: only a path from the root of the same origin, so
 * that a message never opens another site; the notifications page for anything else.
 */
export function targetURL(path: string, origin: string): string {
  const fallback = new URL("/notifications", origin).href;
  if (!path.startsWith("/") || path.startsWith("//") || path.startsWith("/\\")) return fallback;
  const url = new URL(path, origin);
  return url.origin === origin ? url.href : fallback;
}

/**
 * The notification to show for a message's text, or for one that cannot be read (no payload, not the server's
 * document): the browser requires a notification for every message (`userVisibleOnly`), so that one has the
 * instance's host as its title and opens the notifications page.
 */
export function notificationFor(text: string | null, origin: string): ShownNotification {
  let payload: unknown = null;
  try {
    payload = text === null ? null : JSON.parse(text);
  } catch {
    payload = null;
  }
  if (!isPayload(payload)) {
    return { title: new URL(origin).host, options: { data: { url: targetURL("/notifications", origin) } } };
  }
  return {
    title: payload.title,
    options: {
      tag: payload.notification_id,
      lang: payload.lang,
      dir: "auto",
      data: { url: targetURL(payload.path, origin) },
    },
  };
}

/** The page that tapping a shown notification opens: the URL in its data, if it is the worker's origin's. */
export function clickedURL(data: unknown, origin: string): string {
  const url =
    typeof data === "object" && data !== null && "url" in data && typeof data.url === "string" ? data.url : "";
  try {
    const parsed = new URL(url);
    if (parsed.origin === origin) return parsed.href;
  } catch {
    // not a URL
  }
  return targetURL("/notifications", origin);
}
