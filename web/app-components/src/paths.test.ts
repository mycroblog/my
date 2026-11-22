// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  autoDeletePath,
  eventPath,
  hashtagPath,
  notificationPath,
  tagPath,
  voiceRoomPath,
  type NotificationTarget,
} from "./paths";

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

  test("an event's notification opens the event, whatever else it carries, and stays without one", () => {
    for (const type of [
      "event_invited",
      "event_schedule_confirmed",
      "event_changed",
      "event_canceled",
      "event_reminder",
      "event_started",
      "event_room_unavailable",
    ]) {
      expect([type, of(type, { account: sora, status: post, event_id: "e1" })]).toEqual([type, "/events/e1"]);
      expect([type, of(type, { account: sora, status: post })]).toEqual([type, "/notifications"]);
    }
  });

  test("a voice chat room's start and an invitation to it open the room, whatever else they carry", () => {
    for (const type of ["voice_room_started", "voice_room_invited"]) {
      expect([type, of(type, { voice_room_id: "r1" })]).toEqual([type, voiceRoomPath("r1")]);
      expect([type, of(type, { account: sora, status: post, voice_room_id: "r1" })]).toEqual([type, "/voice/r1"]);
    }
  });

  test("a voice chat room's notification without the room stays on the notifications", () => {
    for (const type of ["voice_room_started", "voice_room_invited"])
      expect([type, of(type, { account: sora })]).toEqual([type, "/notifications"]);
  });

  test("an event's address is the app's event screen, its ID encoded", () => {
    expect(eventPath("01a1f1")).toBe("/events/01a1f1");
    expect(eventPath("a/b")).toBe(`/events/${encodeURIComponent("a/b")}`);
  });

  test("a room's address is the app's room screen, its ID encoded", () => {
    expect(voiceRoomPath("01a1f1")).toBe("/voice/01a1f1");
    expect(voiceRoomPath("a/b")).toBe(`/voice/${encodeURIComponent("a/b")}`);
  });

  test("a notice with nothing to open stays on the notifications", () => {
    for (const type of [
      "moderation_notice",
      "report_resolved",
      "service_closure",
      "scheduled_status_failed",
      "supporter_payment_failed",
      "admin_webhook_disabled",
    ])
      expect([type, of(type)]).toEqual([type, "/notifications"]);
  });

  test("the notice of the server's auto-delete limit opens the auto-delete settings, whatever else it carries", () => {
    expect(of("auto_delete_limit")).toBe("/settings/account/auto-delete");
    expect(of("auto_delete_limit", { account: sora, status: post })).toBe("/settings/account/auto-delete");
    expect(autoDeletePath).toBe("/settings/account/auto-delete");
  });
});
