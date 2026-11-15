// SPDX-License-Identifier: AGPL-3.0-only

// The addresses of an account's profile and of a post in the web client: the server's own (ADR-0018), which its HTML
// shell serves with their link previews (internal/interface/web/preview.go), so that a link the client draws is the
// link the reader shares. A local account is `/@user`, a remote one `/@user@domain`, a post is under its author, and a
// hashtag is `/tags/{name}`.

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
