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

/** The address of the held-back notifications of the account of `id`. */
export const heldOfPath = (id: string) => `${heldPath}/${id}`;

/** What a notification reads to decide where it leads: its kind and what it is about. */
export type NotificationTarget = {
  readonly type: string;
  readonly status?: { readonly id: string; readonly account: Handle } | undefined;
  readonly account?: Handle | undefined;
  readonly account_move?: { readonly target?: Handle | undefined } | undefined;
  readonly announcement?: { readonly id: string } | undefined;
};

const requests: ReadonlySet<string> = new Set(["reply_request", "boost_request", "quote_request"]);

/**
 * Where a notification leads, the one place that decides it for every list of them: what waits for the reader's
 * approval for a request to reply, boost or quote; the follow requests for one; the post it is about; the account an
 * account moved to; the announcement; the account that acted; else the notifications. An event's and a voice chat
 * room's lead to the notifications until the client has their screens.
 */
export function notificationPath(n: NotificationTarget): string {
  if (requests.has(n.type)) return waitingPath;
  // The events domain adds /events/{id}, and the voice domain /voice/{id}, of the redesign; until then the
  // notifications of an event and of a room lead nowhere else, rather than to a screen the client lacks.
  if (n.type.startsWith("event_") || n.type.startsWith("voice_room_")) return "/notifications";
  if (n.type === "follow_request") return followRequestsPath;
  if (n.status !== undefined) return statusPath(n.status);
  if (n.type === "account_moved" && n.account_move?.target !== undefined) return accountPath(n.account_move.target);
  if (n.announcement !== undefined) return `/announcements/${n.announcement.id}`;
  if (n.account !== undefined) return accountPath(n.account);
  return "/notifications";
}

/** The pages of this client that a push's own page may name: an account's, a post's, an announcement's. */
const pushPages = [/^\/@[^/?#\\]+(?:\/status\/[^/?#\\]+)?$/, /^\/announcements\/[^/?#\\]+$/];

/**
 * Where a click on a Web Push notification leads, as notificationPath decides for the lists, from what the push carries
 * (its kind, and the page the server names): what waits for the reader's approval, the follow requests, the
 * notifications for an event's or a room's; else the server's page when it is one of this client's (an account, a post,
 * an announcement), and the notifications for any other, which this client has no screen for. The server still names
 * the pages of the client before this one (/settings/follow-requests, /moderation/…), hence the kind decides first;
 * #3148 has it name this client's pages.
 */
export function pushTargetPath(push: { readonly type: string; readonly path: string }): string {
  if (requests.has(push.type)) return waitingPath;
  if (push.type.startsWith("event_") || push.type.startsWith("voice_room_")) return "/notifications";
  if (push.type === "follow_request") return followRequestsPath;
  return pushPages.some((page) => page.test(push.path)) ? push.path : "/notifications";
}
