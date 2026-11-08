// SPDX-License-Identifier: AGPL-3.0-only
// Cloudflare Web Analytics on the production site (ADR-0150 §7): the beacon of the dashboard's manual snippet, in the
// head of every page of every version and locale. docs-site-deploy.yml alone passes the site token to the build
// (DOCS_SITE_CF_BEACON_TOKEN, a repository variable, since the token is public in every page); a build without it (a
// local build, the merge queue's check, a pull request's preview) has no beacon. The beacon sets no cookie and stores
// nothing in the browser; the notice page (src/content/docs/analytics.mdx) tells readers what it sends.
// A Content Security Policy of the site would allow script-src https://static.cloudflareinsights.com and connect-src
// https://cloudflareinsights.com.

/** The script of Cloudflare Web Analytics, as the dashboard's snippet loads it. */
const beaconScript = "https://static.cloudflareinsights.com/beacon.min.js";

/** An entry of Starlight's `head`. */
interface HeadEntry {
  tag: "script";
  attrs: Record<string, string | boolean>;
}

/** The entries of Starlight's `head` for the site token `token`: the beacon, or nothing without a token. */
export function analyticsHead(token: string | undefined): HeadEntry[] {
  if (token === undefined || token === "") {
    return [];
  }
  // The dashboard gives a token of hex digits; anything else is a mistake in the variable and fails the build.
  if (!/^[0-9A-Za-z]{16,64}$/.test(token)) {
    throw new Error("DOCS_SITE_CF_BEACON_TOKEN is not a site token of Cloudflare Web Analytics");
  }
  return [{ tag: "script", attrs: { defer: true, src: beaconScript, "data-cf-beacon": JSON.stringify({ token }) } }];
}
