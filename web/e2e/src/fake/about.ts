// SPDX-License-Identifier: AGPL-3.0-only

// What the fake's instance says of itself on its about page (FR-705, FR-718, FR-725, FR-810, ADR-0072 §13): its
// description and contact, its rules, its documents (GET /instance/documents/{kind}), the external reviews it sends to,
// the most days it keeps a post, and the domains it moderates (GET /instance/domain_actions) for whom it lists them.
// By default, what the fake's instance has always said: its summary, its rules, its status page, its terms and its
// privacy policy, and no list of domains.

import { day, page } from "./common";
import type { Handlers, Refusal, Schema } from "./spec";

type Audience = Schema<"InstanceAudience">;

/** What the fake's instance says on its about page. */
export type FakeAboutOptions = {
  /**
   * `full`: everything the page can show besides, its description in paragraphs, its contact (the administrator's
   * account and an address), the external reviews in use with what they send, the most days a post is kept, and the
   * commerce disclosure; `bare`: nothing but its name and domain, its source code and version (no summary, rules,
   * status page nor document).
   */
  says?: "full" | "bare";
  /** Who may read the moderated domains: nobody by default. */
  domains?: Audience;
  /** The instance (`GET /instance`) is never answered (`waits`), or always fails (`fails`). */
  instance?: "waits" | "fails";
};

/** What the about handlers take from the fake. */
export type AboutContext = {
  readonly options: FakeAboutOptions | undefined;
  readonly started: number;
  readonly origin: string;
  readonly visitor: boolean;
  /** The instance's summary, as the fake writes it (long with `longSummary`). */
  readonly summary: string;
  /** The rules in effect. */
  readonly rules: Schema<"InstanceRule">[];
  /** The status page's address. */
  readonly statusPage: string;
  /** The account of the administrator, the contact of a `full` instance. */
  readonly contactAccount: () => Schema<"Account">;
  readonly problem: (status: number, title: string, more?: Partial<Schema<"Problem">>) => Refusal;
  readonly notFound: () => Refusal;
};

/** The instance's fields of its about page. */
export type AboutInstance = Pick<
  Schema<"Instance">,
  | "short_description"
  | "description"
  | "status_page_url"
  | "terms_url"
  | "privacy_policy_url"
  | "commerce_disclosure_url"
  | "rules"
  | "contact"
  | "external_reviews"
  | "external_review_scope"
  | "auto_delete_limit"
  | "domain_actions_visibility"
>;

const description =
  "We started this server in the spring of 2024 for the people of the river town and anyone who likes it here. " +
  "It runs on one small machine in a shared rack, paid for by its supporters.\n\n" +
  "Post in any language. The local timeline is for what happens around the river; the moderators keep it calm and " +
  "answer reports within a day.\n\n" +
  "Questions about the server go to the contact below, not to the moderators' inbox.";

/** The documents' HTML, as the server renders and sanitizes it, its links absolute. */
const documentsAt = (origin: string): Readonly<Record<Schema<"InstanceDocumentKind">, string>> => ({
  terms:
    "<h2>Who may use this server</h2><p>Anyone of 16 or older may hold one account here. By signing up, you agree " +
    `to these terms and to the <a href="${origin}/about">server's rules</a>.</p><h2>Your content</h2><p>What you post ` +
    "stays yours. You let the server store it, show it to the people you chose, and send it to the servers of your " +
    "followers.</p><ul><li>We remove what breaks the rules.</li><li>We may suspend an account that keeps breaking " +
    "them, after a warning.</li><li>You may export your data and leave at any time.</li></ul><h2>Changes</h2><p>We " +
    "announce a change of these terms 30 days before it applies.</p>",
  privacy_policy:
    "<h2>What we keep</h2><p>Your email address, your posts and the media you upload, and the address you signed in " +
    "from for 14 days.</p><h2>Who we share it with</h2><p>Nobody but the servers your posts are delivered to. We " +
    "sell nothing and run no tracker.</p><table><thead><tr><th>Data</th><th>Kept for</th></tr></thead><tbody>" +
    "<tr><td>Sign-in addresses</td><td>14 days</td></tr><tr><td>Deleted posts</td><td>30 days</td></tr>" +
    "<tr><td>Backups</td><td>90 days</td></tr></tbody></table><h2>Contact</h2><p>Write to " +
    '<a href="mailto:admin@example.invalid">admin@example.invalid</a>.</p>',
  commerce_disclosure:
    "<table><tbody><tr><th>Seller</th><td>River Town Social</td></tr><tr><th>Address</th><td>Disclosed without " +
    "delay on request</td></tr><tr><th>Price</th><td>As shown on each plan, tax included</td></tr><tr><th>Payment" +
    "</th><td>Card, through the payment provider</td></tr><tr><th>Cancellation</th><td>Any time; the plan ends at " +
    "the end of its period</td></tr></tbody></table>",
});

/** The moderated domains, newest first, one of them obfuscated, and the public comments. */
const domainActions: Schema<"InstanceDomainAction">[] = (
  [
    { domain: "spam.example", kind: "suspend", comment: "Spam from new accounts every day." },
    { domain: "loud.example", kind: "limit", comment: "Too many unmarked sensitive posts." },
    { domain: "ha***ss.example", kind: "suspend", comment: "Harassment of our members." },
    { domain: "bots.example", kind: "limit" },
    {
      domain: "a-very-long-subdomain.of-a-server-whose-name-runs-long.example",
      kind: "suspend",
      comment: "Shares what our members post privately.",
    },
  ] as const
).map((a, n) => ({ ...a, digest: n.toString(16).padStart(64, "0") }));

/** The about page's fields of the instance, and the handlers of its documents and moderated domains. */
export function aboutHandlers(c: AboutContext): { handlers: Handlers; instance: AboutInstance } {
  const says = c.options?.says;
  const visibility: Audience = c.options?.domains ?? "off";
  const bare = says === "bare";
  const full = says === "full";
  const published: Readonly<Record<Schema<"InstanceDocumentKind">, boolean>> = {
    terms: !bare,
    privacy_policy: !bare,
    commerce_disclosure: full,
  };
  const revisedAt = new Date(c.started - 40 * day).toISOString();
  const documents = documentsAt(c.origin);

  const instance: AboutInstance = {
    ...(bare ? {} : { short_description: c.summary, status_page_url: c.statusPage }),
    ...(published.terms ? { terms_url: `${c.origin}/about/terms` } : {}),
    ...(published.privacy_policy ? { privacy_policy_url: `${c.origin}/about/privacy` } : {}),
    ...(published.commerce_disclosure ? { commerce_disclosure_url: `${c.origin}/about/commerce` } : {}),
    rules: bare ? [] : c.rules,
    ...(full
      ? {
          description,
          contact: { account: c.contactAccount(), email: "admin@example.invalid" },
          external_reviews: ["signup.check", "media.check", "inbound.status", "local.status"],
          external_review_scope: { local_status_account_age_days: 7, inbound_status_unfollowed_direct: true },
          auto_delete_limit: {
            max_age_days: 365,
            allow_pinned: true,
            kept_pins: 3,
            allow_bookmarked: true,
            allow_favourited: false,
            allow_media: true,
            allow_polls: false,
            allow_articles: true,
            allow_visibilities: false,
            allow_thresholds: true,
          },
        }
      : {}),
    domain_actions_visibility: visibility,
  };

  const handlers: Handlers = {
    "GET /instance/documents/{kind}": ({ params }) =>
      published[params.kind]
        ? { status: 200, body: { kind: params.kind, html: documents[params.kind], revised_at: revisedAt } }
        : c.notFound(),

    "GET /instance/domain_actions": ({ query }) => {
      if (visibility === "off") return c.notFound();
      if (visibility === "signed_in" && c.visitor)
        return c.problem(401, "Sign in to see this", { type: "/api/v1/problems/unauthenticated" });
      return { status: 200, body: page(domainActions, query, (a) => a.domain).body };
    },
  };
  return { handlers, instance };
}
