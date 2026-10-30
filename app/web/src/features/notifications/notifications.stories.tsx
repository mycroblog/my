// SPDX-License-Identifier: AGPL-3.0-only
// The notifications (/notifications; FR-205) in the states of ADR-0112 §1, with every kind of notification: from
// other people, from the server to the user, and from the server to its staff.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import {
  abdulrahman,
  bob,
  carol,
  dmitri,
  haruka,
  maximiliana,
  newsBot,
  pictureHandlers,
  pollPost,
  post,
  viewer,
  viewerPost,
} from "../../test/postStories";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";

/** An announcement as the API answers it, published at 2026-10-21T00:00:00Z and unread, with `overrides`. */
function announcementFixture(id: string, overrides: Partial<Schemas["Announcement"]> = {}): Schemas["Announcement"] {
  return {
    id,
    title: "Maintenance on Sunday",
    content: "<p>Maintenance on Sunday.</p>",
    published_at: "2026-10-21T00:00:00Z",
    updated_at: "2026-10-21T00:00:00Z",
    read: false,
    ended: false,
    ...overrides,
  };
}

type Notification = Schemas["Notification"];

/** The ID of the `n`th notification; a higher one is newer. */
function notificationId(n: number): string {
  return `0192f0c4-7a8e-7000-8000-0000000d${String(n).padStart(4, "0")}`;
}

/** Notification `n` of `type`, `n` × 13 minutes before the newest, with `fields`. */
function notification(n: number, type: Schemas["NotificationType"], fields: Partial<Notification> = {}): Notification {
  return {
    id: notificationId(100 - n),
    type,
    created_at: new Date(Date.UTC(2026, 9, 28, 12, 0) - n * 13 * 60_000).toISOString(),
    ...fields,
  };
}

/** The viewer's post that the favourites, boosts and reactions are about. */
const liked = viewerPost;

/** A post of the viewer with a poll that ended. */
const closedPoll: Schemas["Status"] = {
  ...pollPost,
  account: viewer,
  ...(pollPost.poll !== undefined && { poll: { ...pollPost.poll, expired: true, voted: true, own_votes: [0] } }),
};

/** The notifications from other people: one of each kind, with the favourites and boosts of one post grouped. */
const social: Notification[] = [
  notification(0, "mention", {
    account: maximiliana,
    status: post(90, maximiliana, {
      content:
        '<p><span class="h-card"><a href="https://my.example/@alice" class="u-url mention">@<span>alice</span></a></span> Are you coming to the bakery club on Saturday? We are trying rye this time.</p>',
      mentions: [{ id: viewer.id, username: "alice", acct: "alice", url: viewer.url }],
      in_reply_to_id: liked.id,
    }),
  }),
  notification(1, "favourite", { account: bob, status: liked }),
  notification(2, "favourite", { account: carol, status: liked }),
  notification(3, "favourite", { account: haruka, status: liked }),
  notification(4, "favourite", { account: dmitri, status: liked }),
  notification(5, "reaction", { account: abdulrahman, status: liked, reaction: { name: "🍂" } }),
  notification(6, "boost", { account: haruka, status: liked }),
  notification(7, "boost", { account: newsBot, status: liked }),
  notification(8, "quote", {
    account: bob,
    status: post(91, bob, {
      content: "<p>This is exactly the kind of morning I want this week.</p>",
      quote: { state: "accepted", quoted_status: liked },
    }),
  }),
  notification(9, "follow", { account: abdulrahman }),
  notification(10, "follow_request", { account: { ...dmitri, locked: true } }),
  notification(11, "follow_accepted", { account: carol }),
  notification(12, "poll_closed", { account: viewer, status: closedPoll }),
  notification(13, "voice_room_started", { account: bob, voice_room_id: "0192f0c4-7a8e-7000-8000-0000000e0001" }),
  notification(14, "voice_room_invited", { account: haruka, voice_room_id: "0192f0c4-7a8e-7000-8000-0000000e0002" }),
  notification(15, "account_moved", {
    account: carol,
    account_move: {
      target: { ...carol, id: "0192f0c4-7a8e-7000-8000-0000000000c9", acct: "carol@new.example" },
      outcome: "followed",
    },
  }),
  // A new post of an account whose new posts the viewer chose to be notified of (its profile's bell).
  notification(16, "status", {
    account: haruka,
    status: post(92, haruka, { content: "<p>The first persimmons of the year are on the market today.</p>" }),
  }),
];

const at = "2026-10-28T09:00:00Z";

const subscription = {
  id: "0192f0c4-7a8e-7000-8000-0000000000f1",
  plan_id: "0192f0c4-7a8e-7000-8000-0000000000f2",
  plan_name: "Gold supporter",
  interval: "month",
  price: { amount: "500", currency: "JPY" },
  started_at: "2026-10-01T00:00:00Z",
} as const;

const emojiRequest = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e1",
  kind: "create",
  state: "pending",
  shortcode: "sourdough_loaf",
  created_at: at,
} as const;

/** The notifications from the server to the user: moderation, reports, suspensions, emoji, supporters, closure. */
const fromServer: Notification[] = [
  // An announcement of the administrators, by its title (ADR-0125 §2).
  notification(0, "announcement", {
    id: notificationId(101),
    announcement: announcementFixture("0192f0c4-7a8e-7000-8000-0000000000fa", {
      title: "Maintenance on Sunday from 02:00 to 04:00 UTC",
    }),
  }),
  notification(0, "moderation_notice", {
    moderation_action: {
      id: "0192f0c4-7a8e-7000-8000-0000000000f1",
      kind: "limit",
      kind_label: "Limit",
      reason_category: "spam",
      reason_category_label: "Spam",
      public_reason: "Repeated promotional posts in the local timeline.",
      status_ids: [],
      state: "active",
      issued_at: at,
      expires_at: "2026-11-04T09:00:00Z",
      appealable: true,
    },
  }),
  notification(1, "appeal_decided", {
    moderation_action: {
      id: "0192f0c4-7a8e-7000-8000-0000000000f3",
      kind: "limit",
      kind_label: "Limit",
      reason_category: "spam",
      reason_category_label: "Spam",
      status_ids: [],
      state: "lifted",
      issued_at: at,
      lifted_at: at,
      appealable: false,
      appeal: {
        id: "0192f0c4-7a8e-7000-8000-0000000000f4",
        text: "These were posts about our club's events, not advertising.",
        state: "overturned",
        submitted_at: at,
        decided_at: at,
        decision_reason: "We agree: the posts were about a community event.",
      },
    },
  }),
  notification(2, "report_resolved", {
    report: {
      id: "0192f0c4-7a8e-7000-8000-0000000000f2",
      category: "spam",
      comment: "",
      forwarded: false,
      filed_at: at,
      target_account: dmitri,
      status_ids: [],
      rule_ids: [],
      state: "resolved",
    },
  }),
  notification(3, "relationships_severed", {
    relationship_severance: { kind: "domain_suspension", domain: "bad.example", followers_lost: 12, following_lost: 3 },
  }),
  notification(4, "relationships_severed", {
    account: carol,
    relationship_severance: { kind: "account_suspension", followers_lost: 1, following_lost: 0 },
  }),
  notification(5, "custom_emoji_request_decided", {
    custom_emoji_request: { ...emojiRequest, state: "approved" },
  }),
  notification(6, "custom_emoji_request_decided", {
    custom_emoji_request: {
      ...emojiRequest,
      kind: "image",
      state: "rejected",
      reason: "Too close to the existing :bread: emoji.",
      decided_at: at,
    },
  }),
  notification(7, "supporter_payment_failed", {
    supporter_subscription: { ...subscription, state: "past_due", grace_ends_at: "2026-11-04T09:00:00Z" },
  }),
  notification(8, "supporter_plan_retired", {
    supporter_subscription: { ...subscription, state: "canceling" },
  }),
  notification(9, "supporter_subscription_ended", {
    supporter_subscription: { ...subscription, state: "revoked", end_reason: "reversed" },
  }),
  notification(10, "service_closure", {
    service_closure: { change: "announced", closes_at: "2027-03-31T00:00:00Z", export_until: "2027-04-30T00:00:00Z" },
  }),
  notification(11, "service_closure", {
    service_closure: { change: "rescheduled", closes_at: "2027-06-30T00:00:00Z", export_until: "2027-07-31T00:00:00Z" },
  }),
  notification(12, "service_closure", { service_closure: { change: "withdrawn" } }),
  notification(13, "supporter_service_closure", {
    service_closure: { closes_at: "2027-03-31T00:00:00Z", export_until: "2027-04-30T00:00:00Z" },
    supporter_subscription: { ...subscription, state: "canceling" },
  }),
];

/** The notifications from the server to its staff. */
const toStaff: Notification[] = [
  notification(0, "admin_report", { report_id: "0192f0c4-7a8e-7000-8000-0000000000e9" }),
  notification(1, "admin_sign_up", { account: { ...maximiliana, id: "0192f0c4-7a8e-7000-8000-0000000000ea" } }),
  notification(2, "admin_appeal", { appeal_id: "0192f0c4-7a8e-7000-8000-0000000000f4" }),
  notification(3, "admin_webhook_disabled"),
  notification(4, "admin_custom_emoji_request", { custom_emoji_request: emojiRequest }),
  notification(5, "admin_account_moved", { account_move: { target: { ...carol, acct: "carol@new.example" } } }),
  notification(6, "admin_service_closure_due", {
    service_closure: { closes_at: "2026-10-28T00:00:00Z", export_until: "2026-11-28T00:00:00Z" },
  }),
  notification(7, "admin_error_group", {
    error_group: { change: "created", group_id: "0192f0c4-7a8e-7000-8000-0000000000eb" },
  }),
  notification(8, "admin_error_group", {
    error_group: { change: "regressed", group_id: "0192f0c4-7a8e-7000-8000-0000000000ec" },
  }),
  notification(9, "admin_error_group", { error_group: { change: "others", count: 14 } }),
];

/**
 * GET /notifications answering `items` (those of the asked types), and GET /notifications/marker with the newest
 * `unread` of them unread.
 */
function notifications(items: Notification[], unread = 3) {
  const lastRead = items[unread]?.id ?? notificationId(0);
  return [
    http.get(`${apiBase}/notifications`, ({ request }) =>
      withViewer(request, () => {
        const types = new URL(request.url).searchParams.getAll("types");
        return HttpResponse.json({
          items: items.filter((item) => types.length === 0 || types.includes(item.type)),
        } satisfies Schemas["NotificationPage"]);
      }),
    ),
    http.get(`${apiBase}/notifications/marker`, ({ request }) =>
      withViewer(request, () =>
        HttpResponse.json({ last_read_id: lastRead, unread_count: unread } satisfies Schemas["NotificationMarker"]),
      ),
    ),
  ];
}

const meta = {
  title: "Screens/Notifications/List",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: { path: "/notifications", signedIn: true, handlers: [...pictureHandlers, ...notifications(social)] },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** One of each kind from other people, the newest three unread, with the favourites and boosts of one post grouped. */
export const Normal: Story = {};

/** The notices of the server to the user: moderation, a report, suspensions, custom emoji, supporters, closure. */
export const FromTheServer: Story = answering(...notifications(fromServer, 2));

/** The notices of the server to its staff. */
export const ToTheStaff: Story = answering(...notifications(toStaff, 0));

/** Only the mentions. */
export const MentionsOnly: Story = { parameters: { screen: { path: "/notifications?filter=mentions" } } };

/** Only the new posts of the accounts the viewer subscribes to. */
export const NewPostsOnly: Story = { parameters: { screen: { path: "/notifications?filter=posts" } } };

/** Nothing yet. */
export const Empty: Story = answering(...notifications([], 0));

/** The notifications have not arrived yet. */
export const Loading: Story = loadingOn("get", "/notifications");

/** The server failed to answer the notifications. */
export const ServerError: Story = failingOn("get", "/notifications");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** The notices of the server in Japanese, whose sentences are longer. */
export const FromTheServerJapanese: Story = { ...inLocale("ja"), ...answering(...notifications(fromServer, 2)) };

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

/** The notices of the server at 320 CSS px. */
export const FromTheServerNarrow: Story = { ...narrow, ...answering(...notifications(fromServer, 2)) };
