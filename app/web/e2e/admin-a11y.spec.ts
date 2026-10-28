// SPDX-License-Identifier: AGPL-3.0-only
// The accessibility regression suite of the staff's screens (ADR-0015, ADR-0027, ADR-0058, web-client/14-admin.md §14):
// every moderation and server administration screen passes axe without scrolling sideways in every width × locale
// project (the nightly E2E_MATRIX=full runs them all), the list of screens follows the moderation and admin
// routes, and a moderator handles a report with the keyboard alone — the moderation page, the queue, the report,
// the target's history on its moderation page, back to the report, an action that resolves it, and back to the
// queue — with focus on the new page's h1 after every navigation. `vite preview`
// has no API, so each test plays the server of a signed-in moderator with every admin permission with
// page.route; names change with the locale, so the structure is checked by role and by what the server sends.
import { readdirSync } from "node:fs";
import type { Locator, Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, servedImage, test } from "./fixtures";

type Schemas = components["schemas"];

/** The signed-in moderator's account, as GET /accounts/verify_credentials answers, with every admin permission. */
const moderator = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000001",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  badges: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
  discoverable: true,
  indexable: true,
  ai_opt_out: false,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: [
    "manage_reports",
    "manage_accounts",
    "view_audit",
    "manage_domains",
    "manage_invites",
    "manage_jobs",
    "manage_settings",
    "manage_rules",
    "manage_custom_emojis",
    "manage_webhooks",
    "manage_roles",
    "manage_announcements",
    "manage_finance",
    "manage_supporters",
    "manage_oauth_apps",
    "manage_service_closure",
    "manage_errors",
    "manage_login_providers",
  ],
};

/** The reported account: a long display name, local. */
const target: Schemas["Account"] = {
  ...moderator,
  id: "0192f0c4-7a8e-7000-8000-000000000021",
  username: "maximiliana",
  acct: "maximiliana",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000021",
  display_name: "Maximiliana Theodora Wolfharte",
  url: "https://my.example/@maximiliana",
};

/** The reporter: the instance actor of a server on a long domain. */
const longDomain = "the-longest-community-server-name-in-the-fediverse.social.example.com";
const reporter: Schemas["Account"] = {
  ...target,
  id: "0192f0c4-7a8e-7000-8000-000000000031",
  username: "instance.actor",
  acct: `instance.actor@${longDomain}`,
  domain: longDomain,
  uri: `https://${longDomain}/actor`,
  display_name: "",
  url: `https://${longDomain}/actor`,
};

const postId = "0192f0c4-7a8e-7000-8000-0000000000b1";
const post: Schemas["Status"] = {
  id: postId,
  uri: `https://my.example/ap/statuses/${postId}`,
  url: `https://my.example/@maximiliana/status/${postId}`,
  account: target,
  content: "<p>Buy cheap followers at https://spam.example/now — limited offer!</p>",
  spoiler_text: "",
  sensitive: false,
  visibility: "public",
  language: "en",
  created_at: "2026-10-16T08:00:00Z",
  media_attachments: [],
  mentions: [],
  tags: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
};

const reportId = "0192f0c4-7a8e-7000-8000-0000000c0002";
/** Another open report, on the same target, that stays open. */
const otherReportId = "0192f0c4-7a8e-7000-8000-0000000c0001";

function reportOf(id: string, overrides: Partial<Schemas["AdminReportDetail"]> = {}): Schemas["AdminReportDetail"] {
  return {
    id,
    category: "spam",
    comment:
      "This account posts the same advertisement in reply to everyone who mentions the server, many times a day.",
    forwarded: false,
    filed_at: "2026-10-16T09:00:00Z",
    state: "open",
    state_changed_at: "2026-10-16T09:00:00Z",
    reporter_account_id: reporter.id,
    reporter_account: reporter,
    uri: `https://${longDomain}/flags/1`,
    target_account_id: target.id,
    target_account: target,
    status_ids: [postId],
    rule_ids: [],
    statuses: [{ status: post }],
    notes: [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000e0001",
        author_account_id: moderator.id,
        body: "Same as last week.",
        created_at: "2026-10-16T10:00:00Z",
      },
    ],
    target_history: { reports_count: 2, actions_count: 1 },
    ...overrides,
  };
}

/** The target's earlier moderation action, in its history. */
const warning: Schemas["AdminAccountAction"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000d0000",
  account_id: target.id,
  kind: "warn",
  reason_category: "spam",
  public_reason: "Posts the same advertisement in reply to everyone who mentions the server.",
  issuer: "moderator",
  issued_by_account_id: moderator.id,
  notify_target: true,
  state: "active",
  issued_at: "2026-10-09T00:00:00Z",
};

const appealId = "0192f0c4-7a8e-7000-8000-0000000f0001";
const appeal: Schemas["AdminAppeal"] = {
  id: appealId,
  account_id: target.id,
  text: `I only posted the link to my own blog: https://birds.example/${"very-long-path-".repeat(6)}end.`,
  state: "pending",
  submitted_at: "2026-10-16T09:00:00Z",
  action: {
    ...warning,
    id: "0192f0c4-7a8e-7000-8000-0000000d0101",
    kind: "suspend",
    issued_at: "2026-10-15T09:00:00Z",
  },
};

const registration = {
  user_id: "0192f0c4-7a8e-7000-8000-0000000000b2",
  account_id: "0192f0c4-7a8e-7000-8000-0000000000c2",
  username: "bob",
  state: "pending",
  since: "2026-10-16T09:00:00Z",
  email: "bob.with.a.long.address@birds.example",
  email_verified: true,
  reason: `I would like to post about birds. My blog is at https://birds.example/${"very-long-path-".repeat(6)}end.`,
  submitted_at: "2026-10-16T09:00:00Z",
};

const domainAction = {
  id: "0192f0c4-7a8e-7000-8000-0000000000a2",
  domain: longDomain,
  kind: "limit",
  public_comment:
    "Spam from every account of this server, reported many times by people here, with no answer from its moderators.",
  private_note: "See the reports.",
  obfuscate: true,
  state: "active",
  issued_at: "2026-10-12T08:00:00Z",
  expires_at: "2026-11-11T08:00:00Z",
};

const signupBlock = {
  id: "0192f0c4-7a8e-7000-8000-0000000001a2",
  kind: "email_domain",
  value: "xn--r8jz45g.jp",
  severity: "reject",
  comment: "Disposable addresses used for many accounts that posted the same advertisement on the same day.",
  state: "active",
  created_at: "2026-10-12T08:00:00Z",
};

const auditEntry = {
  id: "0192f0c4-7a8e-7000-8000-0000000a0004",
  actor_kind: "user",
  actor_account_id: moderator.id,
  actor_account: moderator,
  action: "account_action_issued",
  target_kind: "account",
  target_id: target.id,
  target_label: `@${target.acct}`,
  occurred_at: "2026-10-17T09:30:00Z",
};

/** A local custom emoji with a long category and other names, as GET /admin/custom_emojis lists it. */
const customEmoji: Schemas["AdminCustomEmoji"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e2",
  shortcode: "blobcat_with_a_very_long_name",
  local: true,
  uri: "https://my.example/ap/emojis/0192f0c4-7a8e-7000-8000-0000000000e2",
  url: servedImage(
    "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGN4ViH3nxLMMGrAqAGjBgwXAwAvJnsfj06SzAAAAABJRU5ErkJggg==",
  ),
  static_url: servedImage(
    "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGN4ViH3nxLMMGrAqAGjBgwXAwAvJnsfj06SzAAAAABJRU5ErkJggg==",
  ),
  enabled: true,
  visible_in_picker: true,
  category: "Blobs and other round creatures of the fediverse",
  aliases: ["cat", "kitty", "round_cat_of_the_fediverse"],
  created_by_deleted: true,
  created_at: "2026-10-20T00:00:00Z",
  updated_at: "2026-10-20T00:00:00Z",
};

/** A webhook that the server disabled, with a long description and URL, as GET /admin/webhooks lists it. */
const webhook: Schemas["AdminWebhook"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e3",
  url: `https://hooks.example/services/${"a-very-long-path-segment-".repeat(4)}end`,
  description: "Moderation chat of the staff, with every report and appeal",
  events: ["report.created", "report.resolved", "appeal.created", "status.created"],
  state: "disabled",
  state_reason: "failing",
  state_changed_at: "2026-10-20T00:00:00Z",
  failure: { consecutive_failures: 20, first_failed_at: "2026-10-17T00:00:00Z" },
  created_at: "2026-10-16T00:00:00Z",
  updated_at: "2026-10-16T00:00:00Z",
};

/** The roles, as GET /admin/roles lists them: the built-in ones, and one with a long key and a badge. */
const roles: Schemas["AdminRole"][] = [
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000f7",
    key: "admin",
    position: 1000,
    permissions: [
      "manage_reports",
      "manage_accounts",
      "manage_domains",
      "manage_rules",
      "manage_settings",
      "manage_roles",
      "manage_invites",
      "view_audit",
      "manage_jobs",
      "manage_login_recovery",
      "manage_custom_emojis",
      "manage_webhooks",
      "manage_announcements",
      "manage_finance",
      "manage_supporters",
      "view_finance",
      "create_custom_emojis",
    ],
  },
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000f8",
    key: "moderator",
    position: 100,
    permissions: ["manage_reports", "manage_accounts", "view_audit"],
  },
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000f9",
    key: "weekend-volunteers-of-the-community-help-desk",
    position: 50,
    permissions: ["manage_reports", "manage_jobs"],
    badge: { label: "Weekend volunteer of the desk", color: "yellow" },
  },
];

/** The announced closure of the service, with an operators' message of one long word, as GET /admin/closure shows it. */
const announcedClosure: Schemas["AdminServiceClosure"] = {
  state: "announced",
  closes_at: "2099-01-01T00:00:00Z",
  export_until: "2099-02-01T00:00:00Z",
  message: `Thank you for the years${"-and-more".repeat(8)}`,
  announced_at: "2026-10-20T00:00:00Z",
  short_notice: false,
};

/** What closing would end, as GET /admin/closure/check shows it. */
const closureCheck: Schemas["AdminServiceClosureCheck"] = {
  local_accounts: 12,
  delivery_inboxes: 340,
  estimated_deletes: 4080,
  live_supporter_subscriptions: 3,
  pending_migrations: 2,
  retention_holds: 1,
};

/** A published announcement with a long first line and a link, as GET /admin/announcements lists it. */
const announcement: Schemas["AdminAnnouncement"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e6",
  text: `# Scheduled maintenance of the database ${"and of the media storage ".repeat(3)}\n\nSee [the status page](https://status.example/).`,
  content: `<h2>Scheduled maintenance of the database ${"and of the media storage ".repeat(3)}</h2><p>See <a href="https://status.example/">the status page</a>.</p>`,
  state: "published",
  published_at: "2026-10-20T00:00:00Z",
  ends_at: "2026-10-27T00:00:00Z",
  created_at: "2026-10-20T00:00:00Z",
  updated_at: "2026-10-20T00:00:00Z",
};

/** A failing delivery of the webhook, as GET /admin/webhooks/{id}/deliveries lists it. */
const webhookDelivery: Schemas["AdminWebhookDelivery"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e4",
  message_id: "0192f0c4-7a8e-7000-8000-0000000000e5",
  event_type: "report.created",
  status: "failing",
  attempts: 8,
  last_attempt: { outcome: "http_error", status_code: 503, duration_ms: 120, attempted_at: "2026-10-20T00:00:00Z" },
  created_at: "2026-10-19T00:00:00Z",
};

/** A user-registered OAuth app with a long name, as GET /admin/oauth_clients lists it. */
const oauthClient: Schemas["AdminOAuthClient"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000a2",
  client_id: "e2e-client",
  name: "A-very-long-named-app-for-testing-that-wraps-at-320-pixels",
  registration: "user",
  confidential: false,
  scope: "read write",
  owner_id: moderator.id,
  disabled: false,
  created_at: "2026-10-22T00:00:00Z",
};

/** An OIDC login provider with a long name and issuer, as GET /admin/login_providers lists it. */
const loginProvider: Schemas["AdminLoginProvider"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000f2",
  kind: "oidc",
  display_name: "A-very-long-named-login-provider-for-testing-that-wraps-at-320-pixels",
  issuer: `https://id.${longDomain}/realms/staff`,
  client_id: "e2e-login-client",
  scopes: [],
  allow_signup: false,
  allow_link: true,
  claims: [],
  organizations: [],
  guilds: [],
  state: "enabled",
  state_changed_at: "2026-10-22T00:00:00Z",
  callback_url: "https://my.example/auth/external/0192f0c4-7a8e-7000-8000-0000000000f2/callback",
  linked_users: 3,
  created_at: "2026-10-22T00:00:00Z",
  updated_at: "2026-10-22T00:00:00Z",
};

/** A joined relay on a long domain that a domain action suspends, as GET /admin/relays lists it. */
const relay: Schemas["AdminRelay"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e6",
  protocol: "litepub",
  url: `https://relay.${longDomain}/actor`,
  inbox_url: `https://relay.${longDomain}/inbox`,
  receive: true,
  state: "accepted",
  state_changed_at: "2026-10-20T00:00:00Z",
  actor_uri: `https://relay.${longDomain}/actor`,
  taken_last_day: 12345,
  dropped_last_day: 678,
  domain_suspended: true,
  created_at: "2026-10-19T00:00:00Z",
};

const instance = {
  domain: longDomain,
  state: "unavailable",
  since: "2026-10-10T08:00:00Z",
  failure_days: 7,
  last_failure_day: "2026-10-10",
};

/**
 * A rule written in Arabic, which reads right to left, with a long hint in English, as GET /admin/rules lists it: the
 * administrator writes each once, in any language.
 */
const rule: Schemas["AdminRule"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000f2",
  position: 1,
  text: "كن لطيفا",
  hint: "No harassment, no slurs, and no piling on someone who is having a bad day.",
  created_at: "2026-10-16T00:00:00Z",
  revised_at: "2026-10-16T00:00:00Z",
};

/** A usable invite without a limit that the target created as a member, as GET /admin/invites lists it. */
const invite: Schemas["AdminInvite"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000f3",
  code: "ABCDEFGHJKLMNPQR",
  url: "https://my.example/auth/sign_up?invite=ABCDEFGHJKLMNPQR",
  uses: 2,
  state: "usable",
  creator_kind: "member",
  created_by: target,
  created_by_deleted: false,
  expires_at: "2026-10-23T00:00:00Z",
  created_at: "2026-10-16T00:00:00Z",
};

/** The instance settings, as GET /admin/settings lists them: every one at its default. */
const instanceSettings: Schemas["AdminSettingList"] = {
  items: [
    {
      key: "registration_mode",
      kind: "enum",
      value: "approval",
      default: "approval",
      stored: false,
      choices: ["open", "approval", "invite", "closed"],
    },
    { key: "default_locale", kind: "locale", value: "en", default: "en", stored: false },
    {
      key: "local_timeline_access",
      kind: "enum",
      value: "everyone",
      default: "everyone",
      stored: false,
      choices: ["off", "logged_in", "everyone"],
    },
    {
      key: "federated_timeline_access",
      kind: "enum",
      value: "everyone",
      default: "everyone",
      stored: false,
      choices: ["off", "logged_in", "everyone"],
    },
    {
      key: "remote_media_policy",
      kind: "enum",
      value: "none",
      default: "none",
      stored: false,
      choices: ["none", "avatars", "all"],
    },
    {
      key: "remote_media_cache_ttl",
      kind: "int",
      value: "86400",
      default: "86400",
      stored: false,
      minimum: 60,
      maximum: 31536000,
    },
    { key: "authorized_fetch", kind: "bool", value: "false", default: "false", stored: false },
    {
      key: "domain_actions_visibility",
      kind: "enum",
      value: "none",
      default: "none",
      stored: false,
      choices: ["none", "users", "all"],
    },
    {
      key: "domain_actions_rationale_visibility",
      kind: "enum",
      value: "none",
      default: "none",
      stored: false,
      choices: ["none", "users", "all"],
    },
    { key: "search_indexing", kind: "enum", value: "deny", default: "deny", stored: false, choices: ["allow", "deny"] },
    {
      key: "ai_usage",
      kind: "enum",
      value: "deny",
      default: "deny",
      stored: false,
      choices: ["allow", "deny_training", "deny"],
    },
    { key: "indexable_default", kind: "bool", value: "false", default: "false", stored: false },
    { key: "security_contact", kind: "uri", value: "", default: "", stored: false },
    { key: "link_cards_enabled", kind: "bool", value: "true", default: "true", stored: false },
    {
      key: "invite_creators",
      kind: "enum",
      value: "permitted",
      default: "permitted",
      stored: false,
      choices: ["everyone", "permitted"],
    },
    { key: "member_invite_max_active", kind: "int", value: "5", default: "5", stored: false, minimum: 0, maximum: 100 },
    {
      key: "member_invite_max_uses",
      kind: "int",
      value: "10",
      default: "10",
      stored: false,
      minimum: 1,
      maximum: 1000,
    },
    {
      key: "member_invite_max_expires_in",
      kind: "int",
      value: "2592000",
      default: "2592000",
      stored: false,
      minimum: 3600,
      maximum: 31536000,
    },
  ],
};

const deadJob = {
  id: "0192f0c4-7a8e-7000-8000-0000000000d1",
  type: "federation.deliver",
  state: "dead",
  payload: JSON.stringify({
    delivery_id: "0192f0c4-7a8e-7000-8000-0000000000aa",
    inbox: `https://${longDomain}/users/someone-with-a-long-name/inbox`,
  }),
  idempotency_key: "deliver:0192f0c4-7a8e-7000-8000-0000000000aa",
  attempts: 9,
  last_error: "502 Bad Gateway",
  created_at: "2026-10-13T10:00:00Z",
  failed_at: "2026-10-17T10:00:00Z",
};

/** An open error group with a long place, type and message, as GET /admin/error_groups lists it. */
const errorGroup: Schemas["ErrorGroup"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e1",
  fingerprint: "0123456789abcdef0123456789abcdef",
  state: "open",
  state_since: "2026-10-20T00:00:00Z",
  source: "server",
  kind: "panic",
  where: "client_api GET /api/v1/accounts/{id}/statuses/{status_id}/favourited_by",
  type: "*github.com/mycroblog/my/internal/adapter/db/postgres.constraintViolationError",
  message: "runtime error: invalid memory address or nil pointer dereference in a very long message that goes on",
  count_24h: 3,
  count_7d: 12,
  first_seen_at: "2026-10-20T00:00:00Z",
  last_seen_at: "2026-10-21T05:30:00Z",
  first_version: "0.3.0",
  last_version: "0.3.1",
};

/** The request of the group's latest occurrence, which the detail shows. */
const errorRequestId = "01JB0000000000000000000000";

/** The group with its latest occurrence and hourly counts, as GET /admin/error_groups/{id} answers. */
const errorGroupDetail: Schemas["ErrorGroupDetail"] = {
  ...errorGroup,
  sample: {
    type: "*fmt.wrapError > *github.com/mycroblog/my/internal/adapter/db/postgres.constraintViolationError",
    message: "runtime error: invalid memory address or nil pointer dereference",
    stack:
      "goroutine 1 [running]:\ngithub.com/mycroblog/my/internal/app/status.(*Service).FavouritedByWithAVeryLongFunctionName(...)\n\tgithub.com/mycroblog/my/internal/app/status/favourited_by.go:42 +0x32",
    request_id: errorRequestId,
    trace_id: "4bf92f3577b34da6a3ce929d0e0e4736",
    version: "0.3.1",
    role: "all",
    occurred_at: "2026-10-21T05:30:00Z",
  },
  hourly_counts: [
    { hour: "2026-10-20T00:00:00Z", count: 9, last_version: "0.3.0" },
    { hour: "2026-10-21T05:00:00Z", count: 3, last_version: "0.3.1" },
  ],
  events: [],
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/**
 * How long the lists of reports take once an action has resolved one: a slow server, so that a queue shown from
 * its cache before the list comes again would show the resolved report (#464).
 */
const slowListMs = 1500;

/** Plays the API of a signed-in moderator with every admin permission; returns the account actions taken. */
async function moderatorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let reports = [reportOf(reportId), reportOf(otherReportId, { filed_at: "2026-10-15T09:00:00Z", comment: "" })];
  let actions = [warning];
  let slowLists = false;
  const issued: string[] = [];
  const adminAccount = (): Schemas["AdminAccountDetail"] => ({
    id: target.id,
    account: target,
    local: true,
    state: "active",
    restrictions: [],
    registration_state: "approved",
    // Invited by the moderator through a member invite, which the page names and links.
    registration: {
      submitted_at: "2026-10-01T00:00:00Z",
      state_changed_at: "2026-10-01T01:00:00Z",
      invite_id: invite.id,
      invite: { id: invite.id, creator_kind: "member", created_by: moderator, created_by_deleted: false },
    },
    email_verified: true,
    roles: [],
    actions,
    reports,
  });
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, {
      access_token: "e2e-token",
      token_type: "Bearer",
      expires_in: 3600,
      scope: "read write follow admin",
    }),
  );
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (request.headers()["authorization"] !== "Bearer e2e-token" && path !== "/instance")
      return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "open" },
        federation: { mode: "both" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials" || key === `GET /accounts/${moderator.id}`)
      return json(route, moderator);
    if (key === `GET /accounts/${target.id}`) return json(route, target);
    if (key === "GET /preferences") {
      return json(route, {
        locale,
        time_zone: "UTC",
        default_visibility: "public",
        default_post_language: "und",
        require_media_description: false,
        timeline_languages: [],
      });
    }
    if (key === "GET /admin/counts") {
      return json(route, {
        open_reports: reports.filter((item) => item.state === "open").length,
        pending_registrations: 1,
        pending_appeals: 1,
      });
    }
    if (key === "GET /admin/reports") {
      if (slowLists) await new Promise((resolve) => setTimeout(resolve, slowListMs));
      const state = url.searchParams.get("state");
      return json(route, { items: reports.filter((item) => state === null || item.state === state) });
    }
    const report = /^GET \/admin\/reports\/([^/]+)$/.exec(key);
    if (report) {
      const found = reports.find((item) => item.id === report[1]);
      return found === undefined
        ? json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404)
        : json(route, found);
    }
    if (key === "GET /admin/accounts")
      return json(route, {
        items: [
          {
            id: target.id,
            account: target,
            local: true,
            state: "active",
            restrictions: [],
            registration_state: "approved",
          },
        ],
      });
    if (key === `GET /admin/accounts/${target.id}`) return json(route, adminAccount());
    if (key === `GET /admin/accounts/${target.id}/actions`) return json(route, { items: actions });
    if (key === `POST /admin/accounts/${target.id}/actions`) {
      const body = request.postDataJSON() as Schemas["CreateAccountActionRequest"];
      issued.push(`${body.kind} ${body.report_id ?? ""}`);
      const action: Schemas["AdminAccountAction"] = {
        id: "0192f0c4-7a8e-7000-8000-0000000d0001",
        account_id: target.id,
        kind: body.kind,
        reason_category: body.reason_category,
        issuer: "moderator",
        issued_by_account_id: moderator.id,
        notify_target: body.notify_target,
        state: "active",
        issued_at: "2026-10-17T00:00:00Z",
        ...(body.report_id === undefined ? {} : { report_id: body.report_id }),
      };
      actions = [action, ...actions];
      reports = reports.map((item) =>
        item.id === body.report_id
          ? { ...item, state: "resolved", outcome: "actioned", state_changed_at: "2026-10-17T00:00:00Z" }
          : item,
      );
      slowLists = true;
      return json(route, action);
    }
    if (key === "GET /admin/appeals") {
      const state = url.searchParams.get("state");
      return json(route, { items: state === null || state === "pending" ? [appeal] : [] });
    }
    if (key === `GET /admin/appeals/${appealId}`) return json(route, appeal);
    if (key === "GET /admin/registrations") return json(route, { items: [registration] });
    if (key === "GET /admin/domain_actions") return json(route, { items: [domainAction] });
    if (key === "GET /admin/signup_blocks") return json(route, { items: [signupBlock] });
    if (key === "GET /admin/audit_log") return json(route, { items: [auditEntry] });
    if (key === "GET /admin/instances") return json(route, { items: [instance] });
    if (key === `GET /admin/instances/${longDomain}`) return json(route, instance);
    if (key === `GET /admin/instances/${longDomain}/availability_events`) {
      return json(route, {
        items: [
          {
            id: "0192f0c4-7a8e-7000-8000-0000000000e1",
            kind: "marked_unavailable",
            reason: "failure_days",
            occurred_at: "2026-10-10T08:00:00Z",
          },
        ],
      });
    }
    if (key === "GET /admin/queue")
      return json(route, {
        adapter: "db",
        pending_listing: true,
        counts: [{ state: "dead", type: deadJob.type, count: 1 }],
      });
    if (key === "GET /admin/jobs") return json(route, { items: [deadJob] });
    if (key === "GET /admin/error_groups") return json(route, { items: [errorGroup] });
    if (key === `GET /admin/error_groups/${errorGroup.id}`) return json(route, errorGroupDetail);
    if (key === "GET /admin/instance_texts") return json(route, { items: [documentText, instanceName] });
    if (key === "GET /admin/theme") return json(route, adminTheme);
    if (key === "GET /admin/settings") return json(route, instanceSettings);
    if (key === "GET /admin/rules") return json(route, { items: [rule] });
    if (key === "GET /admin/invites") return json(route, { items: [invite] });
    if (key === "GET /admin/custom_emojis")
      return json(route, { items: url.searchParams.get("local") === "false" ? [] : [customEmoji] });
    if (key === "GET /admin/custom_emoji_requests") return json(route, { items: [] });
    if (key === "GET /admin/webhooks") return json(route, { items: [webhook] });
    if (key === "GET /admin/announcements") return json(route, { items: [announcement] });
    if (key === "GET /admin/closure") return json(route, announcedClosure);
    if (key === "GET /admin/closure/check") return json(route, closureCheck);
    if (key === `GET /admin/webhooks/${webhook.id}`) return json(route, webhook);
    if (key === `GET /admin/webhooks/${webhook.id}/deliveries`) return json(route, { items: [webhookDelivery] });
    if (key === "GET /admin/relays") return json(route, { items: [relay] });
    if (key === "GET /admin/oauth_clients") return json(route, { items: [oauthClient] });
    if (key === "GET /admin/login_providers") return json(route, { items: [loginProvider] });
    if (key === "GET /admin/roles") return json(route, { items: roles });
    if (key === "GET /admin/cost_items") return json(route, { items: [costItem] });
    if (key === "GET /admin/cost_totals") return json(route, costTotals);
    if (key === "GET /finance/cost_totals") {
      return json(route, {
        ...costTotals,
        periods: costTotals.periods.map((period) => ({ ...period, items: [] })),
        viewer: { item_names: false, item_memos: false, income: "totals" },
      });
    }
    if (key === "GET /admin/supporter_program") return json(route, supporterProgram);
    if (key === "GET /admin/payment_status") return json(route, paymentStatus);
    if (key === "GET /admin/supporter_plans") return json(route, { items: [supporterPlan] });
    if (key === "GET /admin/supporter_subscriptions") return json(route, { items: [supporterSubscription] });
    if (key === "GET /admin/billing_imports") return json(route, { items: [] });
    if (key === "GET /admin/income_totals") return json(route, incomeTotals);
    if (key === "GET /admin/income_movements") return json(route, { items: [incomeMovement] });
    if (/^GET \/admin\/accounts\/[^/]+\/roles$/.test(key))
      return json(route, { items: key.includes(moderator.id) ? roles.slice(0, 1) : [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return issued;
}

/** A yearly cost with a long provider name, as GET /admin/cost_items lists it. */
const costItem: Schemas["CostItem"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000a1",
  schedule: "years",
  interval: 1,
  starts_on: "2026-04-10",
  name: "example.com",
  provider: "the-registrar-of-a-very-long-domain-name-that-does-not-wrap.example",
  category: "domain",
  memo: "",
  prices: [
    { effective_on: "2026-04-10", price: { amount: "1650", currency: "JPY" }, revised_at: "2026-04-10T00:00:00Z" },
  ],
  created_at: "2026-04-10T00:00:00Z",
};

/** The totals of the costs screen: one month in two currencies with an estimate. */
const costTotals: Schemas["CostTotals"] = {
  granularity: "month",
  basis: "amortized",
  periods: [
    {
      label: "2026-10",
      starts_on: "2026-10-01",
      ends_on: "2026-10-31",
      totals: [
        { amount: "140", currency: "JPY" },
        { amount: "31.24", currency: "USD" },
      ],
      breakdown: [{ provider: "registrar", category: "domain", amount: { amount: "140", currency: "JPY" } }],
      estimated: true,
    },
  ],
  rules: { week_start: "monday", sources: [{ source: "aws", provider: "AWS", time_zone: "UTC" }] },
};

/** The income of the month as the administrators see it, with a plan's members. */
const incomeTotals: Schemas["AdminIncomeTotals"] = {
  granularity: "month",
  basis: "amortized",
  periods: [
    {
      label: "2026-10",
      starts_on: "2026-10-01",
      ends_on: "2026-10-31",
      gross: [{ amount: "12000", currency: "JPY" }],
      fee: [{ amount: "432", currency: "JPY" }],
      net: [{ amount: "11568", currency: "JPY" }],
      balance: [
        { amount: "11428", currency: "JPY" },
        { amount: "-31.24", currency: "USD" },
      ],
    },
  ],
  plans: [{ plan_id: "0192f0c4-7a8e-7000-8000-0000000000c1", name: "Coffee", members: 1 }],
  rules: {
    week_start: "monday",
    providers: [{ provider: "stripe", time_zone: "Asia/Tokyo" }],
    amortize_after_days: 31,
  },
};

/** A yearly payment of a supporter plan, as GET /admin/income_movements lists it. */
const incomeMovement: Schemas["IncomeMovement"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000d1",
  provider: "stripe",
  provider_id: "txn_1",
  kind: "payment",
  subscription_id: "0192f0c4-7a8e-7000-8000-0000000000d2",
  account_id: "0192f0c4-7a8e-7000-8000-0000000000d3",
  username: "carol",
  occurred_at: "2026-10-10T03:00:00Z",
  occurred_on: "2026-10-10",
  covers_starts_on: "2026-10-10",
  covers_ends_on: "2027-10-09",
  gross: { amount: "12000", currency: "JPY" },
  fee: { amount: "432", currency: "JPY" },
  net: { amount: "11568", currency: "JPY" },
  recorded_at: "2026-10-10T03:01:00Z",
};

/** A supporter plan, as GET /admin/supporter_plans lists it. */
const supporterPlan: Schemas["AdminSupporterPlan"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000c1",
  name: "Supporter plan of a very long name that does not wrap",
  description: "",
  rank: 1,
  visibility: "public",
  monthly_price: { amount: "500", currency: "JPY" },
  yearly_price: { amount: "5400", currency: "JPY" },
  annual_discount_percent: 10,
  emoji_limit: 5,
  role: { id: "role-supporter", key: "supporter", position: 10, permissions: [] },
  prices_ready: true,
  retired: false,
  members: 1,
  created_at: "2026-10-20T00:00:00Z",
  revised_at: "2026-10-20T00:00:00Z",
};

/** A subscription to that plan, as GET /admin/supporter_subscriptions lists it. */
const supporterSubscription: Schemas["AdminSupporterSubscription"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e1",
  state: "active",
  plan_id: supporterPlan.id,
  plan_name: supporterPlan.name,
  interval: "month",
  price: { amount: "500", currency: "JPY" },
  period_ends_at: "2026-11-20T00:00:00Z",
  account_id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  started_at: "2026-10-20T00:00:00Z",
};

const supporterProgram: Schemas["AdminSupporterProgram"] = {
  enabled: true,
  provider: "stripe",
  configured_provider: "stripe",
  sends_email: true,
  commerce_disclosure: true,
  live_subscriptions: 1,
  can_enable: true,
  warnings: [],
};

const paymentStatus: Schemas["PaymentStatus"] = { provider: "stripe", mode: "test", time_zone: "UTC", warnings: [] };

/** The terms of service, as GET /admin/instance_texts lists them. */
const documentText: Schemas["AdminInstanceText"] = {
  key: "terms",
  body: "# Accounts\n\nBe kind.",
  revised_at: "2026-10-12T08:00:00Z",
};
/** The server's name, written in Arabic (a language without a catalog), which reads right to left in every project. */
const instanceName: Schemas["AdminInstanceText"] = { key: "name", body: "مثال", revised_at: "2026-10-16T00:00:00Z" };

/** The server's theme, as GET /admin/theme answers: a color, a radius, and a custom CSS. */
const adminTheme: Schemas["AdminTheme"] = {
  revision: "0192f0c4-7a8e-7000-8000-0000000000f1",
  revised_at: "2026-10-20T00:00:00Z",
  stylesheet_url: "https://my.example/theme/0192f0c4-7a8e-7000-8000-0000000000f1.css",
  tokens: [
    { token: "color-surface", light: "#fdfdf6", dark: "#05070d" },
    { token: "radius-md", value: "3px" },
  ],
  custom_css: ".brand { font-weight: 600; }",
};

/**
 * Every staff screen, by the name of its route file in src/routes, with an address that opens it and a text of
 * the server's answer that shows once it has loaded (or, for a screen that shows it only as the value of a field,
 * that field's value). A new moderation or admin route fails the test below until it is listed.
 */
const adminPages: Readonly<Record<string, { path: string; ready: string | { value: string } }>> = {
  "moderation.index": { path: "/moderation", ready: "" },
  "admin.index": { path: "/admin", ready: "" },
  "moderation.reports": { path: "/moderation/reports", ready: `@${target.acct}` },
  "moderation.reports_.$id": { path: `/moderation/reports/${reportId}`, ready: "Same as last week." },
  "moderation.accounts": { path: "/moderation/accounts", ready: target.display_name },
  "moderation.accounts_.$id": { path: `/moderation/accounts/${target.id}`, ready: target.display_name },
  "moderation.appeals": { path: "/moderation/appeals", ready: target.display_name },
  "moderation.appeals_.$id": { path: `/moderation/appeals/${appealId}`, ready: target.display_name },
  "moderation.registrations": { path: "/moderation/registrations", ready: registration.email },
  "moderation.domain-actions": { path: "/moderation/domain-actions", ready: domainAction.public_comment },
  "moderation.signup-blocks": { path: "/moderation/signup-blocks", ready: signupBlock.comment },
  "moderation.audit-log": { path: "/moderation/audit-log", ready: auditEntry.target_label },
  "moderation.instances": { path: "/moderation/instances", ready: longDomain },
  "moderation.instances_.$domain": { path: `/moderation/instances/${longDomain}`, ready: longDomain },
  "admin.custom-emojis": { path: "/admin/custom-emojis", ready: customEmoji.category ?? "" },
  "admin.jobs": { path: "/admin/jobs?state=dead", ready: deadJob.type },
  "admin.errors": { path: "/admin/errors", ready: errorGroup.where },
  "admin.errors_.$id": { path: `/admin/errors/${errorGroup.id}`, ready: errorRequestId },
  "admin.webhooks": { path: "/admin/webhooks", ready: webhook.description ?? "" },
  "admin.webhooks_.$id": { path: `/admin/webhooks/${webhook.id}`, ready: webhookDelivery.message_id },
  "admin.closure": { path: "/admin/closure", ready: announcedClosure.message ?? "" },
  "admin.announcements": { path: "/admin/announcements", ready: "the status page" },
  "admin.relays": { path: "/admin/relays", ready: relay.inbox_url },
  "admin.oauth-apps": { path: "/admin/oauth-apps", ready: oauthClient.name },
  "admin.login-providers": { path: "/admin/login-providers", ready: loginProvider.display_name },
  "admin.documents": { path: "/admin/documents", ready: { value: documentText.body } },
  "admin.settings": { path: "/admin/settings", ready: "my.example" },
  "admin.theme": { path: "/admin/theme", ready: { value: "#fdfdf6" } },
  "admin.rules": { path: "/admin/rules", ready: rule.hint ?? "" },
  "admin.roles": { path: "/admin/roles", ready: roles[2]?.key ?? "" },
  "admin.finance": { path: "/admin/finance", ready: costItem.provider },
  "admin.supporters": { path: "/admin/supporters", ready: supporterSubscription.plan_name },
  "moderation.invites": { path: "/moderation/invites", ready: invite.code },
};

test("the suite lists every moderation and admin route", () => {
  const routes = readdirSync(new URL("../src/routes", import.meta.url))
    .filter((name) => /^(moderation|admin)\..+\.tsx$/.test(name) && !name.endsWith(".test.tsx"))
    .map((name) => name.replace(/\.tsx$/, ""));
  expect(Object.keys(adminPages).toSorted()).toEqual(routes.toSorted());
});

for (const [route, { path, ready }] of Object.entries(adminPages)) {
  test.describe(`staff page ${path} (${route})`, () => {
    test.beforeEach(async ({ page }) => {
      await moderatorServer(page);
      await page.goto(path);
      const main = page.getByRole("main");
      await expect(main.getByRole("heading", { level: 1 })).toBeVisible();
      // The pages of the two areas list links only; the other screens show what the server sent.
      if (typeof ready !== "string") await expect(main.getByRole("textbox").first()).toHaveValue(ready.value);
      else if (ready === "") await expect(main.getByRole("link").first()).toBeVisible();
      else await expect(main.getByText(ready).first()).toBeVisible();
    });

    test("has no axe violations", async ({ makeAxeBuilder }) => {
      await expectNoAxeViolations(makeAxeBuilder);
    });

    test("does not scroll horizontally", async ({ page }) => {
      await expectNoHorizontalScroll(page);
    });
  });
}

/** Whether `locator` or an element in it (a radio of a radio group) has focus. */
function hasFocus(locator: Locator): Promise<boolean> {
  return locator.evaluate((element) => element.contains(document.activeElement));
}

/**
 * Presses Tab (Shift+Tab with `backward`) until `locator` has focus, as a keyboard user moves through the page;
 * fails if it is not reached within 200 presses, which would mean it is out of the tab order.
 */
async function tabTo(page: Page, locator: Locator, { backward = false } = {}): Promise<void> {
  const limit = 200;
  await expect(locator).toBeVisible();
  for (let presses = 0; presses < limit; presses++) {
    if (await hasFocus(locator)) return;
    await page.keyboard.press(backward ? "Shift+Tab" : "Tab");
  }
  expect(
    await hasFocus(locator),
    `not reached with ${String(limit)} presses of ${backward ? "Shift+Tab" : "Tab"}`,
  ).toBe(true);
}

/** Asserts the new page's h1 has focus (web-client/README.md Y5), axe passes, and the page does not scroll sideways. */
async function expectPageReached(
  page: Page,
  makeAxeBuilder: Parameters<typeof expectNoAxeViolations>[0],
): Promise<void> {
  await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeFocused();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
}

test.describe("handling a report with the keyboard alone", () => {
  test("opens a report, reads the target's history, takes an action that resolves it, and returns to the queue", async ({
    page,
    makeAxeBuilder,
  }) => {
    // Every step is many presses of Tab, each checked for focus.
    test.setTimeout(120_000);
    const issued = await moderatorServer(page);
    await page.goto("/moderation");
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1 })).toBeVisible();

    // The moderation overview leads to the report queue: its tile of the open reports comes first.
    await tabTo(page, main.locator('a[href="/moderation/reports"]').first());
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/moderation\/reports$/);
    const queue = main.getByRole("table");
    await expect(queue.locator(`a[href="/moderation/reports/${reportId}"]`)).toBeVisible();
    await expectPageReached(page, makeAxeBuilder);

    // The queue opens the report.
    await tabTo(page, queue.locator(`a[href="/moderation/reports/${reportId}"]`));
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/moderation/reports/${reportId}$`));
    await expect(main.getByRole("article")).toBeVisible();
    await expectPageReached(page, makeAxeBuilder);

    // The target's history: its action on the report's page, and its moderation page with its reports and actions.
    await expect(main.getByRole("region").filter({ hasText: warning.public_reason ?? "" })).toBeVisible();
    await tabTo(page, main.locator(`a[href="/moderation/accounts/${target.id}"]`));
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/moderation/accounts/${target.id}$`));
    await expect(main.getByText(warning.public_reason ?? "").first()).toBeVisible();
    await expectPageReached(page, makeAxeBuilder);

    // Back to the report from the account's reports.
    await tabTo(page, main.locator(`a[href="/moderation/reports/${reportId}"]`));
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/moderation/reports/${reportId}$`));
    await expect(main.getByRole("article")).toBeVisible();
    await expectPageReached(page, makeAxeBuilder);

    // An action, chosen with the arrow keys in its radio group, resolves the report (FR-710).
    const handling = main.getByRole("region").last();
    const limit = handling.locator('input[type="radio"][value="limit"]');
    await tabTo(page, handling.getByRole("radiogroup"));
    for (let presses = 0; presses < 20 && !(await limit.isChecked()); presses++) await page.keyboard.press("ArrowDown");
    await expect(limit).toBeChecked();
    await expect(limit).toBeFocused();
    await tabTo(page, handling.locator('form button[type="submit"]'));
    await page.keyboard.press("Enter");
    await expect(handling.getByRole("status").filter({ hasText: /\S/ })).toBeVisible();
    await expect(handling.getByRole("heading", { level: 2 })).toBeFocused();
    expect(issued).toEqual([`limit ${reportId}`]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Back on the queue, the report is gone from the open ones as soon as the queue shows, though the server
    // answers the lists slowly now: the queue is not shown from a cache older than the action.
    await tabTo(page, main.locator('a[href="/moderation/reports"]'), { backward: true });
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/moderation\/reports$/);
    await expect(queue.locator(`a[href="/moderation/reports/${otherReportId}"]`)).toBeVisible();
    expect(await queue.locator(`a[href="/moderation/reports/${reportId}"]`).count()).toBe(0);
    await expect(queue.locator('a[href^="/moderation/reports/"]')).toHaveCount(1);
    await expect(queue.locator(`a[href="/moderation/reports/${reportId}"]`)).toHaveCount(0);
    await expectPageReached(page, makeAxeBuilder);
  });
});
