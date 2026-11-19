// SPDX-License-Identifier: AGPL-3.0-only
/// <reference types="bun" />

import { describe, expect, test } from "bun:test";
import { clickedURL, notificationFor, windowToOpen, type OpenWindow, type Shown } from "./push";

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
    path: "/notifications/0190a5d0-0000-7000-8000-000000000001",
    ...fields,
  });

describe("notificationFor", () => {
  test("shows the server's title, tagged with the notification's ID, and keeps the notification's page on this origin", () => {
    const shown = notificationFor(message(), origin);
    expect(shown.title).toBe("Sora mentioned you");
    expect(shown.options.tag).toBe("0190a5d0-0000-7000-8000-000000000001");
    expect(urlOf(shown)).toBe("https://my.example/notifications/0190a5d0-0000-7000-8000-000000000001");
  });

  test("a message sent twice is tagged alike, so that it replaces itself", () => {
    expect(notificationFor(message(), origin).options.tag).toBe(
      notificationFor(message({ title: "Sora mentioned you again" }), origin).options.tag,
    );
  });

  test("every kind leads to the notification's page, whatever page the message names, on this origin alone", () => {
    for (const [type, path] of [
      ["reply_request", "/@yuki/status/9"],
      ["follow_request", "/settings/follow-requests"],
      ["event_invited", "https://evil.example/@sora"],
      ["mention", "//evil.example/@sora"],
    ])
      expect([type, urlOf(notificationFor(message({ type, path }), origin))]).toEqual([
        type,
        "https://my.example/notifications/0190a5d0-0000-7000-8000-000000000001",
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
      "https://my.example/notifications/0190a5d0-0000-7000-8000-000000000001",
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

describe("the window a click opens its address in", () => {
  const at = (path: string, focused = false) => ({ url: `${origin}${path}`, focused });

  test("is the focused one, else any, else none", () => {
    const home = at("/home");
    const focused = at("/notifications", true);
    expect(windowToOpen([home, focused])).toBe(focused);
    expect(windowToOpen([home])).toBe(home);
    expect(windowToOpen<OpenWindow>([])).toBeUndefined();
  });

  test("is never one on a payment page, focused or not", () => {
    const checkout = at("/payments/pages/payjp/checkout?s=x", true);
    const home = at("/home");
    expect(windowToOpen([checkout, home])).toBe(home);
    expect(windowToOpen([checkout, at("/payments")])).toBeUndefined();
    expect(windowToOpen([at("/paymentsx", true)])?.url).toBe(`${origin}/paymentsx`);
  });
});
