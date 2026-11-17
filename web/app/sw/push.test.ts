// SPDX-License-Identifier: AGPL-3.0-only
/// <reference types="bun" />

import { describe, expect, test } from "bun:test";
import { clickedURL, notificationFor, type Shown } from "./push";

const origin = "https://my.example";

/** The address a click on `shown` opens, as the notification keeps it. */
const urlOf = (shown: Shown): string => (shown.options.data as { url: string }).url;

/** The document the server encrypts for one notification (internal/app/notification PushPayload). */
const message = (fields: Record<string, unknown> = {}) =>
  JSON.stringify({
    notification_id: "0190a5d0-0000-7000-8000-000000000001",
    type: "mention",
    title: "Sora mentioned you",
    lang: "en",
    path: "/@sora@photos.example/status/9",
    ...fields,
  });

describe("notificationFor", () => {
  test("shows the server's title, tagged with the notification's ID, and keeps the page a click opens on this origin", () => {
    const shown = notificationFor(message(), origin);
    expect(shown.title).toBe("Sora mentioned you");
    expect(shown.options.tag).toBe("0190a5d0-0000-7000-8000-000000000001");
    expect(urlOf(shown)).toBe("https://my.example/@sora@photos.example/status/9");
  });

  test("a message sent twice is tagged alike, so that it replaces itself", () => {
    expect(notificationFor(message(), origin).options.tag).toBe(
      notificationFor(message({ title: "Sora mentioned you again" }), origin).options.tag,
    );
  });

  test("leads where the client opens the kind: a request to what waits, a follow request to the follow requests", () => {
    expect(urlOf(notificationFor(message({ type: "reply_request" }), origin))).toBe(
      "https://my.example/notifications/waiting",
    );
    expect(urlOf(notificationFor(message({ type: "follow_request", path: "/settings/follow-requests" }), origin))).toBe(
      "https://my.example/notifications/follow_requests",
    );
    expect(urlOf(notificationFor(message({ type: "event_invited", path: "/@sora" }), origin))).toBe(
      "https://my.example/notifications",
    );
  });

  test("a page of another origin, or one this client has none of, leads to the notifications on this origin", () => {
    for (const path of ["https://evil.example/@sora", "//evil.example/@sora", "/moderation/reports/1"])
      expect([path, urlOf(notificationFor(message({ path }), origin))]).toEqual([
        path,
        "https://my.example/notifications",
      ]);
  });

  test("a message the worker cannot read still shows, as the server's host, and leads to the notifications", () => {
    for (const text of [null, "", "not json", "[]", JSON.stringify({ title: 3 }), message({ title: undefined })]) {
      const shown = notificationFor(text, origin);
      expect([text, shown.title, urlOf(shown)]).toEqual([text, "my.example", "https://my.example/notifications"]);
    }
  });
});

describe("clickedURL", () => {
  test("opens the address the notification keeps", () => {
    expect(clickedURL({ url: "https://my.example/@sora/status/9" }, origin)).toBe("https://my.example/@sora/status/9");
    expect(clickedURL(notificationFor(message(), origin).options.data, origin)).toBe(
      "https://my.example/@sora@photos.example/status/9",
    );
  });

  test("never opens another origin, and stays on this one when the notification keeps no address", () => {
    for (const data of [
      { url: "https://evil.example/@sora" },
      { url: "//evil.example/" },
      { url: "javascript:alert(1)" },
      { url: 3 },
      {},
      null,
      undefined,
      "https://evil.example/",
    ]) {
      const url = clickedURL(data, origin);
      expect([data, new URL(url).origin]).toEqual([data, origin]);
    }
  });
});
