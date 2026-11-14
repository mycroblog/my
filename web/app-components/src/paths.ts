// SPDX-License-Identifier: AGPL-3.0-only

// The addresses of an account's profile and of a post in the web client: the server's own (ADR-0018), which its HTML
// shell serves with their link previews (internal/interface/web/preview.go), so that a link the client draws is the
// link the reader shares. A local account is `/@user`, a remote one `/@user@domain`, and a post is under its author.

/** What an address of an account reads: its handle. */
type Handle = { readonly acct: string; readonly username: string; readonly domain: string };

/** The handle in an address: `user` for a local account, `user@domain` for a remote one, else its domain alone. */
const handleIn = (account: Handle) => account.acct || account.username || account.domain;

/** The address of an account's profile. */
export const accountPath = (account: Handle) => `/@${handleIn(account)}`;

/** The address of a post, under its author's profile. */
export const statusPath = (status: { readonly id: string; readonly account: Handle }) =>
  `${accountPath(status.account)}/status/${status.id}`;
