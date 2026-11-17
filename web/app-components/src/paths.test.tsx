// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { hashtagPath, notificationPath, pushTargetPath, tagPath, type NotificationTarget } from "./paths";
import { PostContent, ServerDomain } from "./post-content";

describe("tagPath", () => {
  test("is the app's address of a hashtag, its name encoded", () => {
    expect(tagPath("baking")).toBe("/tags/baking");
    expect(tagPath("山")).toBe(`/tags/${encodeURIComponent("山")}`);
  });
});

describe("hashtagPath", () => {
  test("a hashtag's page on this server is the app's hashtag screen", () => {
    expect(hashtagPath("https://my.example/tags/baking", "my.example")).toBe("/tags/baking");
    expect(hashtagPath("https://my.example/tags/baking/", "my.example")).toBe("/tags/baking");
    expect(hashtagPath("https://MY.example/tags/baking", "My.Example")).toBe("/tags/baking");
  });

  test("a hashtag in another script keeps its encoded name", () => {
    const encoded = encodeURIComponent("山");
    expect(hashtagPath(`https://my.example/tags/${encoded}`, "my.example")).toBe(`/tags/${encoded}`);
  });

  test("another server's hashtag, or another page of this server, opens as it is", () => {
    expect(hashtagPath("https://other.example/tags/baking", "my.example")).toBeUndefined();
    expect(hashtagPath("https://my.example.evil/tags/baking", "my.example")).toBeUndefined();
    expect(hashtagPath("https://my.example/@yuki", "my.example")).toBeUndefined();
    expect(hashtagPath("https://my.example/tags/baking/extra", "my.example")).toBeUndefined();
    expect(hashtagPath("https://my.example/tags/", "my.example")).toBeUndefined();
  });

  test("without the server's domain, or with no URL, there is no app address", () => {
    expect(hashtagPath("https://my.example/tags/baking", undefined)).toBeUndefined();
    expect(hashtagPath("not a url", "my.example")).toBeUndefined();
  });
});

describe("a hashtag in a post", () => {
  const html =
    '<p><a href="https://my.example/tags/fediverse" class="mention hashtag" rel="tag">#<span>fediverse</span></a> ' +
    '<a href="https://other.example/tags/cats" class="mention hashtag" rel="tag">#<span>cats</span></a></p>';
  const render = (domain: string | undefined) =>
    renderToStaticMarkup(
      <ServerDomain value={domain}>
        <PostContent html={html} lang="en" />
      </ServerDomain>,
    ).replace(/ class="[^"]*"/g, "");

  /** The opening tag of the link whose text is `#name`. */
  const link = (markup: string, name: string) =>
    new RegExp(`<a [^>]*>#<span>${name}</span></a>`).exec(markup)?.[0] ?? "";

  test("of this server opens the app's hashtag screen in place; another server's opens in a new tab", () => {
    const markup = render("my.example");
    expect(link(markup, "fediverse")).toContain('href="/tags/fediverse"');
    expect(link(markup, "fediverse")).not.toContain('target="_blank"');
    expect(link(markup, "cats")).toContain('href="https://other.example/tags/cats"');
    expect(link(markup, "cats")).toContain('target="_blank"');
  });

  test("before the server's domain is known, opens as the server wrote it", () => {
    const markup = render(undefined);
    expect(link(markup, "fediverse")).toContain('href="https://my.example/tags/fediverse"');
    expect(link(markup, "fediverse")).toContain('target="_blank"');
  });
});

describe("notificationPath", () => {
  const sora = { acct: "sora@photos.example", username: "sora", domain: "photos.example" } as const;
  const yuki = { acct: "yuki", username: "yuki", domain: "my.example" } as const;
  const post = { id: "9", account: yuki };
  const of = (type: string, fields: Omit<NotificationTarget, "type"> = {}) => notificationPath({ type, ...fields });

  test("a notification about a post opens the post", () => {
    for (const type of [
      "mention",
      "boost",
      "favourite",
      "reaction",
      "quote",
      "poll_closed",
      "status",
      "interaction_accepted",
      "interaction_rejected",
    ])
      expect([type, of(type, { account: sora, status: post })]).toEqual([type, "/@yuki/status/9"]);
  });

  test("a follow, an accepted follow and a new account open the account that acted", () => {
    for (const type of ["follow", "follow_accepted", "admin_sign_up"])
      expect([type, of(type, { account: sora })]).toEqual([type, "/@sora@photos.example"]);
  });

  test("a follow request opens the follow requests, and a request to reply, boost or quote what waits for approval", () => {
    expect(of("follow_request", { account: sora })).toBe("/notifications/follow_requests");
    for (const type of ["reply_request", "boost_request", "quote_request"])
      expect([type, of(type, { account: sora, status: post })]).toEqual([type, "/notifications/waiting"]);
  });

  test("a move opens the account moved to, and an announcement its page", () => {
    expect(of("account_moved", { account: yuki, account_move: { target: sora } })).toBe("/@sora@photos.example");
    expect(of("announcement", { announcement: { id: "a1" } })).toBe("/announcements/a1");
  });

  test("an event's and a voice chat room's lead nowhere else yet, even with an account or a post", () => {
    for (const type of [
      "event_invited",
      "event_schedule_confirmed",
      "event_changed",
      "event_cancelled",
      "event_reminder",
      "event_started",
      "event_room_unavailable",
      "voice_room_started",
      "voice_room_invited",
    ])
      expect([type, of(type, { account: sora, status: post })]).toEqual([type, "/notifications"]);
  });

  test("a notice with nothing to open stays on the notifications", () => {
    for (const type of [
      "moderation_notice",
      "report_resolved",
      "service_closure",
      "auto_delete_limit",
      "scheduled_status_failed",
      "supporter_payment_failed",
      "admin_webhook_disabled",
    ])
      expect([type, of(type)]).toEqual([type, "/notifications"]);
  });
});

describe("pushTargetPath", () => {
  const of = (type: string, path: string) => pushTargetPath({ type, path });

  test("a request to reply, boost or quote opens what waits for approval, whatever page the server names", () => {
    for (const type of ["reply_request", "boost_request", "quote_request"])
      expect([type, of(type, "/@yuki/status/9")]).toEqual([type, "/notifications/waiting"]);
  });

  test("a follow request opens the follow requests, even where the server names the page of the client before", () => {
    expect(of("follow_request", "/settings/follow-requests")).toBe("/notifications/follow_requests");
    expect(of("follow_request", "/@sora@photos.example")).toBe("/notifications/follow_requests");
  });

  test("an event's and a voice chat room's open the notifications, even with a page of an account or a post", () => {
    for (const type of [
      "event_invited",
      "event_schedule_confirmed",
      "event_changed",
      "event_cancelled",
      "event_reminder",
      "event_started",
      "event_room_unavailable",
      "voice_room_started",
      "voice_room_invited",
      "voice_room_ended",
    ])
      expect([type, of(type, "/@yuki/status/9")]).toEqual([type, "/notifications"]);
  });

  test("any other kind opens the server's page when it is an account's, a post's or an announcement's", () => {
    expect(of("mention", "/@sora@photos.example/status/9")).toBe("/@sora@photos.example/status/9");
    expect(of("favourite", "/@yuki/status/9")).toBe("/@yuki/status/9");
    expect(of("follow", "/@sora@photos.example")).toBe("/@sora@photos.example");
    expect(of("announcement", "/announcements/a1")).toBe("/announcements/a1");
    expect(of("some_new_kind", "/@yuki")).toBe("/@yuki");
  });

  test("a page this client has no screen for, or one off this client, opens the notifications", () => {
    for (const path of [
      "",
      "/",
      "/moderation/reports/1",
      "/settings/preferences",
      "/@yuki/followers",
      "/@yuki/status/9/reactions",
      "/@yuki?tab=media",
      "/@yuki#top",
      "/announcements/",
      "/announcements/a1/extra",
      "https://evil.example/@yuki",
      "//evil.example/@yuki",
      "/@yuki\\@evil.example",
      "javascript:alert(1)",
    ])
      expect([path, of("mention", path)]).toEqual([path, "/notifications"]);
  });
});
