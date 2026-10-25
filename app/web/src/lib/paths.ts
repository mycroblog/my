// SPDX-License-Identifier: AGPL-3.0-only
// The addresses of the app's pages that more than one feature links to. They follow the server's HTML
// shell routes (internal/interface/web, ADR-0028): a profile is `/@username` for a local account and
// `/@username@domain` for a remote one, the same as the `acct` of the client API's Account.

/** The profile page of the account whose `acct` is given. */
export function profilePath(acct: string): string {
  return `/@${acct}`;
}

/** The pages of the accounts that follow an account, and that an account follows. */
export function followListPath(acct: string, list: "followers" | "following"): string {
  return `/@${acct}/${list}`;
}

/**
 * The page of the user's own account status: the moderation actions on the account, and appealing them
 * (FR-715). A `moderation_notice` notification leads there.
 */
export const accountStatusPath = "/settings/account-status";

/** The instance's information page (FR-718), which the shell's third column and the landing page link to. */
export const aboutPath = "/about";

/** The moderation page of one appeal of a moderation action, which an `admin_appeal` notification leads to. */
export function moderationAppealPath(id: string): string {
  return `/moderation/appeals/${id}`;
}

/** The moderation page of one report, which an `admin_report` notification leads to. */
export function moderationReportPath(id: string): string {
  return `/moderation/reports/${id}`;
}

/**
 * The prefixes of the server's own paths (internal/interface/web/handler.go, reservedPrefixes): the client API,
 * the server-rendered pages (sign-in, sign-up, the terms of service and the privacy policy under /auth, OAuth
 * consent under /oauth, the email unsubscribe links under /mail), ActivityPub, media, the instance theme's stylesheets (/theme), and the license notices of the
 * build (/licenses). The app has no route under them.
 */
const serverPathPrefixes = ["/api", "/oauth", "/auth", "/mail", "/.well-known", "/nodeinfo", "/ap", "/media", "/uploads", "/theme", "/licenses"] as const;

/** Whether `pathname` is one of the server's own paths, which a link loads in place of the app, never through the router. */
export function isServerPath(pathname: string): boolean {
  return serverPathPrefixes.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}
