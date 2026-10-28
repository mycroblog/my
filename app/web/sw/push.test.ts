// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from "vitest";
import { clickedURL, notificationFor, targetURL } from "./push.ts";

const origin = "https://my.example";

describe("notificationFor", () => {
  it("shows the server's localized title with the notification as the tag and its page as the target", () => {
    const text = JSON.stringify({
      notification_id: "n1",
      type: "mention",
      title: "Bobさんがあなたをメンションしました",
      lang: "ja",
      path: "/@bob/s1",
    });
    expect(notificationFor(text, origin)).toEqual({
      title: "Bobさんがあなたをメンションしました",
      options: { tag: "n1", lang: "ja", dir: "auto", data: { url: "https://my.example/@bob/s1" } },
    });
  });

  it.each([null, "", "not json", "{}", JSON.stringify({ title: 1 })])(
    "shows the host and the notifications page for the message %j",
    (text) => {
      expect(notificationFor(text, origin)).toEqual({
        title: "my.example",
        options: { data: { url: "https://my.example/notifications" } },
      });
    },
  );
});

describe("targetURL", () => {
  it.each([
    ["/@alice", "https://my.example/@alice"],
    ["/admin/reports/r1", "https://my.example/admin/reports/r1"],
    ["//evil.example/x", "https://my.example/notifications"],
    ["/\\evil.example", "https://my.example/notifications"],
    ["https://evil.example/", "https://my.example/notifications"],
    ["javascript:alert(1)", "https://my.example/notifications"],
  ])("opens %s as %s", (path, want) => {
    expect(targetURL(path, origin)).toBe(want);
  });
});

describe("clickedURL", () => {
  it("opens the URL of the notification's data on the worker's origin only", () => {
    expect(clickedURL({ url: "https://my.example/@bob/s1" }, origin)).toBe("https://my.example/@bob/s1");
    expect(clickedURL({ url: "https://evil.example/" }, origin)).toBe("https://my.example/notifications");
    expect(clickedURL(null, origin)).toBe("https://my.example/notifications");
  });
});
