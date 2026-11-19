// SPDX-License-Identifier: AGPL-3.0-only

// The addresses of an account's profile and of a post in the web client: the server's own (ADR-0018), which its HTML
// shell serves with their link previews (internal/interface/web/preview.go), so that a link the client draws is the
// link the reader shares. A local account is `/@user`, a remote one `/@user@domain`, a post is under its author, and a
// hashtag is `/tags/{name}`. Where a notification leads is decided here alone, for every list of them.

/** What an address of an account reads: its handle. */
type Handle = { readonly acct: string; readonly username: string; readonly domain: string };

/** The handle in an address: `user` for a local account, `user@domain` for a remote one, else its domain alone. */
const handleIn = (account: Handle) => account.acct || account.username || account.domain;

/** The address of an account's profile. */
export const accountPath = (account: Handle) => `/@${handleIn(account)}`;

/** The address of a post, under its author's profile. */
export const statusPath = (status: { readonly id: string; readonly account: Handle }) =>
  `${accountPath(status.account)}/status/${status.id}`;

/** The address of the list of who reacted to a post with one emoji (a reaction's name). */
export const reactorsPath = (status: { readonly id: string; readonly account: Handle }, emoji: string) =>
  `${statusPath(status)}/reactions/${encodeURIComponent(emoji)}`;

/** The address of a hashtag's timeline, of its name without `#`. */
export const tagPath = (name: string) => `/tags/${encodeURIComponent(name)}`;

/**
 * The app's address of a link the server marked as a hashtag (`href`), when it is the hashtag's page on this server,
 * whose serving domain is `domain`: `/tags/{name}`. Undefined for a link to another server, which opens as it is.
 */
export function hashtagPath(href: string, domain: string | undefined): string | undefined {
  if (domain === undefined) return undefined;
  let url: URL;
  try {
    url = new URL(href);
  } catch {
    return undefined;
  }
  const match = /^\/tags\/([^/]+)\/?$/.exec(url.pathname);
  if (url.host !== domain.toLowerCase() || match?.[1] === undefined) return undefined;
  return `/tags/${match[1]}`;
}

/** The address of what waits for the reader's approval: the replies, boosts and quotes of their posts they hold. */
export const waitingPath = "/notifications/waiting";

/** The address of the follow requests that wait for the reader's answer. */
export const followRequestsPath = "/notifications/follow_requests";

/** The address of the notifications the reader's notification policy holds back, by the account that sent them. */
export const heldPath = "/notifications/requests";

/** The address of the notification settings, where the reader chooses what notifies them and how. */
export const notificationSettingsPath = "/settings/notifications";

/** The address of the settings' screen that exports the reader's data: the archive, and the CSV files. */
export const dataExportPath = "/settings/account/export";

/** The address of the settings' screen that moves the reader's account to another account. */
export const accountMovePath = "/settings/account/move";

/** The address of the settings' screen of the automatic deletion of the reader's old posts. */
export const autoDeletePath = "/settings/account/auto-delete";

/** The address of the voice chat rooms the reader sees. */
export const voiceRoomsPath = "/voice";

/** The address of a voice chat room's page (ADR-0100 §9). */
export const voiceRoomPath = (id: string) => `/voice/${encodeURIComponent(id)}`;

/** The address of the form that opens a voice chat room. */
export const voiceOpenPath = "/voice/new";

/** The address of the settings of an open voice chat room, which its host changes. */
export const voiceRoomSettingsPath = (id: string) => `${voiceRoomPath(id)}/settings`;

/** The address of the invitations of an open voice chat room, for its host and co-hosts. */
export const voiceInvitationsPath = (id: string) => `${voiceRoomPath(id)}/invitations`;

/** The address of the settings' screen of the voice chat: the rooms the reader hosted (ADR-0100 §13). */
export const voiceHistoryPath = "/settings/voice";

/** The address of the upcoming events the reader sees (ADR-0161 §9). */
export const eventsPath = "/events";

/** The address of an event's page (ADR-0161 §9). */
export const eventPath = (id: string) => `/events/${encodeURIComponent(id)}`;

/** The address of the form that creates an event (FR-940). */
export const eventNewPath = "/events/new";

/** The address of the form that changes an event, for its organizers. */
export const eventEditPath = (id: string) => `${eventPath(id)}/edit`;

/** The address of the invitations of an event, for its organizers. */
export const eventInvitationsPath = (id: string) => `${eventPath(id)}/invitations`;

/** The address of the co-organizers of an event, which its organizer names. */
export const eventCoorganizersPath = (id: string) => `${eventPath(id)}/coorganizers`;

/** The address of the settings' screen of the reminders of events (ADR-0161 §12). */
export const eventRemindersPath = "/settings/events";

/** The address of the costs page, which the funding meter's month is of (FR-905). */
export const costsPagePath = "/about/costs";

/** The address of the supporters' page: the plans, the tip, and the supporters list (ADR-0070 §13, ADR-0072 §14). */
export const supportersPagePath = "/about/supporters";

/** The address of the settings' screen of the reader's support: their subscription, the plans, the listing. */
export const supporterSettingsPath = "/settings/supporter";

/** The address of the screen that pays a tip, where its checkout returns (ADR-0162 §13). */
export const supporterTipPath = "/settings/supporter/tip";

/** The address of the held-back notifications of the account of `id`. */
export const heldOfPath = (id: string) => `${heldPath}/${id}`;

/** The root of the moderators' screens, which lists those the reader's permissions open. */
export const moderationPath = "/moderation";

/** The root of the server's administration, which lists the screens the reader's permissions open. */
export const adminPath = "/admin";

/** The moderators' queue of reports. */
export const reportQueuePath = "/moderation/reports";

/** The address of a report as a moderator opens it. */
export const reportPath = (id: string) => `${reportQueuePath}/${encodeURIComponent(id)}`;

/** The address of an account's moderation screen, where a moderator reviews it and acts on it. */
export const moderatedAccountPath = (id: string) => `/moderation/accounts/${encodeURIComponent(id)}`;

/** What a notification reads to decide where it leads: its kind and what it is about. */
export type NotificationTarget = {
  readonly type: string;
  readonly status?: { readonly id: string; readonly account: Handle } | undefined;
  readonly account?: Handle | undefined;
  readonly account_move?: { readonly target?: Handle | undefined } | undefined;
  readonly announcement?: { readonly id: string } | undefined;
  readonly voice_room_id?: string | undefined;
  readonly event_id?: string | undefined;
  readonly report_id?: string | undefined;
};

const requests: ReadonlySet<string> = new Set(["reply_request", "boost_request", "quote_request"]);

/**
 * Where a notification leads, the one place that decides it for every list of them: what waits for the reader's
 * approval for a request to reply, boost or quote; the follow requests for one; the post it is about; the account an
 * account moved to; the announcement; the automatic deletion's settings for the server's maximum that will apply to
 * the reader's posts; the voice chat room a room's start or invitation is about; the event an event's notification is
 * about; the account that acted; else the notifications.
 */
export function notificationPath(n: NotificationTarget): string {
  if (requests.has(n.type)) return waitingPath;
  if (n.type === "admin_report") return n.report_id === undefined ? reportQueuePath : reportPath(n.report_id);
  if (n.type === "auto_delete_limit") return autoDeletePath;
  if (n.type.startsWith("voice_room_"))
    return n.voice_room_id === undefined ? "/notifications" : voiceRoomPath(n.voice_room_id);
  if (n.type.startsWith("event_")) return n.event_id === undefined ? "/notifications" : eventPath(n.event_id);
  if (n.type === "follow_request") return followRequestsPath;
  if (n.status !== undefined) return statusPath(n.status);
  if (n.type === "account_moved" && n.account_move?.target !== undefined) return accountPath(n.account_move.target);
  if (n.announcement !== undefined) return `/announcements/${n.announcement.id}`;
  if (n.account !== undefined) return accountPath(n.account);
  return "/notifications";
}
