// SPDX-License-Identifier: AGPL-3.0-only

// The fake's domains, remote servers and audit log (FR-702, FR-704, FR-706, FR-716–FR-718, ADR-0159), as a holder of
// `manage_domains` and `view_audit` reads them under /admin: the actions on remote domains (spam.example suspended,
// noisy.example limited until a month from now, mediahog.example's media rejected by a subscription, old.example's
// suspend lifted and far.example's limit expired), what a suspend would end (the follows with the domain's people in the
// seed), Mastodon's CSV previewed, imported and exported, the allowlist, two block-list subscriptions (one in the draft
// mode with three drafts and its fetches, one automatic whose last three fetches failed), the known remote servers with
// their software (Mastodon, Misskey, snac, Pixelfed, GoToSocial, and two that report none) and their health (deliveries
// failed to reach dead.example, unavailable, flaky.example and gone.example, unavailable), each server's availability
// and deliveries (one of them masked), and the audit log, one entry of every kind of operation, newest first, with what
// the reader does here recorded on top. Every write answers as the server does, and is kept.

import { day, idAt, minute, notFound, page, problem } from "./common";
import type { Answer, Handlers, Schema } from "./spec";

type Action = Schema<"AdminDomainAction">;
type Allow = Schema<"DomainAllow">;
type Subscription = Schema<"AdminDomainListSubscription">;
type Draft = Schema<"AdminDomainListDraft">;
type Fetch = Schema<"AdminDomainListFetch">;
type Server = Schema<"AdminInstanceHealth">;
type Delivery = Schema<"AdminDelivery">;
type Entry = Schema<"AdminAuditEntry">;
type AuditAction = Schema<"AdminAuditAction">;
type TargetKind = Schema<"AdminAuditTargetKind">;
type Account = Schema<"Account">;

/** What the fake holds of domains when it starts: its own (`all`, by default), or nothing. */
export type FakeDomainOptions = "all" | "none";

/** What the domains' handlers take from the fake. */
export type DomainContext = {
  readonly started: number;
  readonly options: FakeDomainOptions | undefined;
  /** The reader's username; their permissions decide what they read. */
  readonly viewer: string;
  readonly permissions: () => readonly Schema<"Permission">[];
  readonly accountOf: (username: string) => Account;
  readonly known: (username: string) => boolean;
  readonly accountId: (username: string) => string;
  readonly accountById: (id: string) => Account | undefined;
  /** The follows between people of the fake now, each as `follower>followee`. */
  readonly follows: () => Iterable<string>;
  /** The domain the fake answers as. */
  readonly host: string;
};

/** What a test does with the fake's domains. */
export type FakeDomains = {
  /** The ID of the fake's domain action (spam, noisy, media, old, far) or subscription (shared, failing), or "none". */
  readonly id: (kind: "action" | "list", key: string) => string;
  /** The audit log as the server holds it now, newest first. */
  readonly audit: () => readonly Entry[];
};

/** The kind of target of each kind of operation, in the API's order (AdminAuditAction). */
const actionTargets: Readonly<Record<AuditAction, TargetKind>> = {
  account_action_issued: "account",
  account_action_lifted: "account",
  domain_action_issued: "domain",
  domain_action_lifted: "domain",
  report_assigned: "report",
  report_unassigned: "report",
  report_resolved: "report",
  report_reopened: "report",
  report_note_added: "report",
  report_forwarded: "report",
  registration_approved: "user",
  registration_rejected: "user",
  setting_changed: "setting",
  text_changed: "setting",
  rule_added: "rule",
  rule_revised: "rule",
  rule_moved: "rule",
  rule_retired: "rule",
  role_granted: "role",
  role_revoked: "role",
  role_created: "role",
  role_updated: "role",
  role_deleted: "role",
  appeal_upheld: "appeal",
  appeal_overturned: "appeal",
  account_deletion_canceled: "account",
  revealed_private_data: "account",
  signup_block_added: "signup_block",
  signup_block_lifted: "signup_block",
  hook_decision_applied: "hook_decision",
  hook_decision_overridden: "hook_decision",
  hook_appeal_upheld: "hook_decision",
  two_factor_disabled: "account",
  password_reset_link_issued: "account",
  email_suppression_lifted: "account",
  account_moved: "account",
  custom_emoji_created: "custom_emoji",
  custom_emoji_image_replaced: "custom_emoji",
  custom_emoji_copied: "custom_emoji",
  custom_emoji_updated: "custom_emoji",
  custom_emoji_enabled: "custom_emoji",
  custom_emoji_disabled: "custom_emoji",
  custom_emoji_deleted: "custom_emoji",
  webhook_created: "webhook",
  webhook_updated: "webhook",
  webhook_deleted: "webhook",
  webhook_enabled: "webhook",
  webhook_disabled: "webhook",
  webhook_secret_rotated: "webhook",
  announcement_created: "announcement",
  announcement_updated: "announcement",
  announcement_published: "announcement",
  announcement_withdrawn: "announcement",
  announcement_deleted: "announcement",
  invite_created: "invite",
  invite_revoked: "invite",
  relay_added: "relay",
  relay_followed: "relay",
  relay_unfollowed: "relay",
  relay_removed: "relay",
  relay_accepted: "relay",
  relay_rejected: "relay",
  status_refetch_queued: "status",
  account_statuses_refetch_queued: "account",
  domain_statuses_refetch_queued: "domain",
  billing_import_queued: "billing_source",
  cost_item_created: "cost_item",
  cost_item_relabeled: "cost_item",
  cost_item_price_revised: "cost_item",
  cost_item_end_changed: "cost_item",
  cost_item_deleted: "cost_item",
  supporter_program_enabled: "supporter_program",
  supporter_program_disabled: "supporter_program",
  supporter_tips_enabled: "supporter_program",
  supporter_tips_revised: "supporter_program",
  supporter_tips_disabled: "supporter_program",
  supporter_plan_created: "supporter_plan",
  supporter_plan_revised: "supporter_plan",
  supporter_plan_retired: "supporter_plan",
  supporter_subscription_ended: "supporter_subscription",
  custom_emoji_request_submitted: "custom_emoji_request",
  custom_emoji_request_withdrawn: "custom_emoji_request",
  custom_emoji_request_approved: "custom_emoji_request",
  custom_emoji_request_rejected: "custom_emoji_request",
  payment_reconcile_queued: "payment_provider",
  oauth_client_disabled: "oauth_client",
  oauth_client_enabled: "oauth_client",
  oauth_client_host_denied: "oauth_client_host",
  oauth_client_host_allowed: "oauth_client_host",
  service_closure_announced: "instance",
  service_closure_withdrawn: "instance",
  service_closure_closed: "instance",
  service_closure_delivery_completed: "instance",
  voice_room_ended: "voice_room",
  login_provider_created: "login_provider",
  login_provider_updated: "login_provider",
  login_provider_deleted: "login_provider",
  login_provider_enabled: "login_provider",
  login_provider_disabled: "login_provider",
  login_provider_secret_rotated: "login_provider",
  link_card_domain_fetch_stopped: "link_card_domain",
  link_card_domain_images_stopped: "link_card_domain",
  link_card_domain_rule_removed: "link_card_domain",
  instance_marked_available: "domain",
  delivery_revealed: "domain",
  account_deliveries_viewed: "account",
  domain_allow_added: "domain_allow",
  domain_allow_removed: "domain_allow",
  domain_list_subscription_created: "domain_list_subscription",
  domain_list_subscription_updated: "domain_list_subscription",
  domain_list_subscription_deleted: "domain_list_subscription",
  domain_list_draft_accepted: "domain_list_subscription",
  domain_list_draft_rejected: "domain_list_subscription",
  domain_list_fetch_queued: "domain_list_subscription",
  event_canceled: "event",
};

/** The name a target of each kind has in the fake's log (its label), and whether it is still there. */
const targets: Readonly<Record<TargetKind, { label: string; exists: boolean }>> = {
  account: { label: "@quickdeals", exists: true },
  status: { label: "https://photos.example/users/sora/statuses/1", exists: true },
  domain: { label: "spam.example", exists: true },
  report: { label: "@quickdeals", exists: true },
  appeal: { label: "@quickdeals", exists: false },
  user: { label: "@newcomer", exists: true },
  role: { label: "moderator @marco", exists: true },
  setting: { label: "registration_mode", exists: true },
  rule: { label: "Be kind to each other.", exists: true },
  signup_block: { label: "email_domain spam.example", exists: false },
  custom_emoji: { label: ":sourdough:", exists: true },
  webhook: { label: "Moderation alerts (hooks.example)", exists: false },
  announcement: { label: "Maintenance on Saturday", exists: true },
  invite: { label: "Q7xK", exists: true },
  relay: { label: "https://relay.example/inbox", exists: true },
  billing_source: { label: "AWS", exists: true },
  cost_item: { label: "Object storage", exists: true },
  supporter_program: { label: "Stripe", exists: true },
  supporter_plan: { label: "Friend of the bakery", exists: true },
  supporter_subscription: { label: "Friend of the bakery @layla", exists: true },
  custom_emoji_request: { label: ":baguette:", exists: true },
  payment_provider: { label: "stripe", exists: true },
  oauth_client: { label: "Toot Viewer (toot.example)", exists: true },
  oauth_client_host: { label: "apps.example", exists: true },
  instance: { label: "service_closure", exists: true },
  voice_room: { label: "Bread talk", exists: true },
  login_provider: { label: "GitHub", exists: true },
  link_card_domain: { label: "tracker.example", exists: true },
  hook_decision: { label: "@quickdeals", exists: true },
  domain_allow: { label: "friend.example", exists: true },
  domain_list_subscription: { label: "Shared block list", exists: true },
  event: { label: "Book swap", exists: true },
};

/** Who did the operations of the fake's log in turn: the reader, another moderator, the command line. */
const actors = ["viewer", "hana", "cli"] as const;

/** The handlers of the domains, the servers and the audit log, and what a test does with them. */
export function domainHandlers(c: DomainContext): { handlers: Handlers; domains: FakeDomains } {
  const iso = (ms: number) => new Date(ms).toISOString();
  const ago = (minutes: number) => c.started - minutes * minute;
  const empty = c.options === "none";
  const may = (permission: Schema<"Permission">) => c.permissions().includes(permission);
  const forbidden = (permission: string) =>
    problem(403, "Forbidden", { detail: `This needs the ${permission} permission.` });
  const keyed = new Map<string, string>();
  let made = 0;
  /** A new ID, as the server makes one now. */
  const newId = () => idAt(Date.now(), 47_000 + (made += 1));

  // ---- The audit log ---------------------------------------------------------------------------------------------

  const audit: Entry[] = [];
  /** Records an operation of the reader's, as the server records it in the same transaction. */
  const record = (action: AuditAction, targetId: string, label: string, exists = true) => {
    const me = c.accountOf(c.viewer);
    audit.unshift({
      id: newId(),
      actor_kind: "user",
      actor_account_id: me.id,
      actor_account: me,
      action,
      target_kind: actionTargets[action],
      target_id: targetId,
      target_label: label,
      target_exists: exists,
      occurred_at: iso(Date.now()),
    });
  };
  if (!empty) {
    // One entry of every kind, an hour apart, the first kind the newest, by the actors in turn.
    (Object.keys(actionTargets) as AuditAction[]).forEach((action, i) => {
      const kind = actionTargets[action];
      // Another server answers a relay; the server itself acts on a review and turns a failing webhook off; the
      // moderators and the command line do the rest, in turn.
      const by =
        action === "relay_accepted" || action === "relay_rejected"
          ? "remote"
          : action === "hook_decision_applied" ||
              action === "webhook_disabled" ||
              action === "service_closure_delivery_completed"
            ? "system"
            : (actors[i % actors.length] ?? "viewer");
      const username = by === "viewer" ? c.viewer : by;
      const person = by === "viewer" || (by === "hana" && c.known("hana")) ? c.accountOf(username) : undefined;
      // An instance text is named by its key, as a setting is by its own.
      const target = action === "text_changed" ? { label: "short_description", exists: true } : targets[kind];
      const id = kind === "account" && c.known("quickdeals") ? c.accountId("quickdeals") : idAt(c.started, 48_000 + i);
      audit.push({
        id: idAt(ago(60 * (i + 1)), 49_000 + i),
        actor_kind: person === undefined ? (by === "viewer" || by === "hana" ? "cli" : by) : "user",
        ...(person === undefined ? {} : { actor_account_id: person.id, actor_account: person }),
        action,
        target_kind: kind,
        target_id: kind === "domain" || kind === "domain_allow" ? target.label : id,
        target_label: target.label,
        target_exists: target.exists,
        occurred_at: iso(ago(60 * (i + 1))),
      });
    });
  }

  // ---- Domain actions --------------------------------------------------------------------------------------------

  const actionOf = (
    key: string,
    domain: string,
    kind: Action["kind"],
    minutesAgo: number,
    more: Partial<Action> = {},
  ): Action => {
    const id = idAt(ago(minutesAgo), 46_000 + keyed.size);
    keyed.set(`action:${key}`, id);
    return {
      id,
      domain,
      kind,
      obfuscate: false,
      state: "active",
      issued_at: iso(ago(minutesAgo)),
      ...more,
    };
  };
  // The newest first.
  const actions: Action[] = empty
    ? []
    : [
        actionOf("spam", "spam.example", "suspend", 90, {
          public_comment: "Spam from every account, no moderation.",
          private_note: "Reported by four people in one day.",
        }),
        actionOf("noisy", "noisy.example", "limit", 3 * 24 * 60, {
          public_comment: "Unmarked graphic content in the federated timeline.",
          expires_at: iso(c.started + 30 * day),
        }),
        actionOf("media", "mediahog.example", "reject_media", 5 * 24 * 60, {
          subscription_id: idAt(ago(40 * 24 * 60), 46_500),
        }),
        actionOf("old", "old.example", "suspend", 60 * 24 * 60, {
          state: "lifted",
          lifted_at: iso(ago(20 * 24 * 60)),
          obfuscate: true,
        }),
        actionOf("far", "far.example", "limit", 90 * 24 * 60, {
          state: "expired",
          expires_at: iso(ago(60 * 24 * 60)),
        }),
      ];

  /** Whether `domain` is `of` or one of its subdomains. */
  const under = (domain: string, of: string) => domain === of || domain.endsWith(`.${of}`);
  /** `written` as the server normalizes a domain; undefined for what is none, or this server's. */
  const domainOf = (written: string): string | undefined => {
    const d = written.trim().toLowerCase().replace(/\.$/, "");
    if (!/^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(d) || d === c.host) return undefined;
    return d;
  };
  const badDomain = () =>
    problem(422, "Unprocessable Content", { detail: "The domain is not valid.", field: "domain" });
  const active = (a: Action) => a.state === "active";

  /** The follows a suspend of `domain` would end, and the local accounts that would lose one. */
  const impactOf = (domain: string) => {
    let follows = 0;
    const locals = new Set<string>();
    for (const edge of c.follows()) {
      const [from = "", to = ""] = edge.split(">");
      const remoteOf = (u: string) => (u.includes("@") ? u.slice(u.indexOf("@") + 1) : undefined);
      const [local, remote] = remoteOf(from) === undefined ? [from, remoteOf(to)] : [to, remoteOf(from)];
      if (remote === undefined || remoteOf(local) !== undefined || !under(remote, domain)) continue;
      follows += 1;
      locals.add(local);
    }
    return { follows, local_accounts: locals.size };
  };

  /** Issues an action as the server does, recorded in the audit log. */
  const issue = (body: Schema<"CreateDomainActionRequest">, domain: string): Action => {
    const now = Date.now();
    const action: Action = {
      id: newId(),
      domain,
      kind: body.kind,
      obfuscate: body.obfuscate,
      state: "active",
      issued_at: iso(now),
      ...(body.public_comment === undefined ? {} : { public_comment: body.public_comment }),
      ...(body.private_note === undefined ? {} : { private_note: body.private_note }),
      ...(body.expires_in === undefined ? {} : { expires_at: iso(now + body.expires_in * 1000) }),
    };
    actions.unshift(action);
    record("domain_action_issued", domain, domain);
    return action;
  };

  // ---- Mastodon's CSV ---------------------------------------------------------------------------------------------

  const truthy = (v: string | undefined) =>
    !["", "0", "f", "false", "off", undefined].includes(v?.trim().toLowerCase());
  /** The kinds a block would issue on `domain` that no action in effect covers. */
  const kindsOf = (b: Schema<"DomainBlock">): Action["kind"][] => {
    const asked: Action["kind"][] = [
      ...(b.severity === "suspend" ? ["suspend" as const] : b.severity === "silence" ? ["limit" as const] : []),
      ...(b.reject_media ? ["reject_media" as const] : []),
      ...(b.reject_reports ? ["reject_reports" as const] : []),
    ];
    return asked.filter((k) => !actions.some((a) => active(a) && a.kind === k && under(b.domain, a.domain)));
  };
  const preview = (csv: string): Schema<"DomainBlockImportRow">[] => {
    const lines = csv.split(/\r?\n/).filter((l) => l.trim() !== "");
    const header = lines[0]?.split(",")[0]?.trim() === "#domain" ? lines[0].split(",").map((h) => h.trim()) : undefined;
    const seen = new Set<string>();
    return lines.slice(header === undefined ? 0 : 1).map((line, i) => {
      const cells = line.split(",");
      const cell = (name: string) => (header === undefined ? undefined : cells[header.indexOf(name)]);
      const written = (header === undefined ? line : (cell("#domain") ?? "")).trim();
      const severity = (cell("#severity") ?? "").trim() || "suspend";
      const n = i + (header === undefined ? 1 : 2);
      const domain = domainOf(written);
      if (domain === undefined && written.trim().toLowerCase() === c.host)
        return { line: n, written_domain: written, status: "own_domain", issue: [] };
      if (domain === undefined || !["suspend", "silence", "noop"].includes(severity))
        return { line: n, written_domain: written, status: "invalid", issue: [] };
      const comment = cell("#public_comment")?.trim();
      const block: Schema<"DomainBlock"> = {
        domain,
        severity: severity as Schema<"DomainBlockSeverity">,
        reject_media: truthy(cell("#reject_media")),
        reject_reports: truthy(cell("#reject_reports")),
        obfuscate: truthy(cell("#obfuscate")),
        ...(comment === undefined || comment === "" ? {} : { public_comment: comment }),
      };
      if (seen.has(domain)) return { line: n, written_domain: written, block, status: "duplicate", issue: [] };
      seen.add(domain);
      const asks = block.severity !== "noop" || block.reject_media || block.reject_reports;
      if (!asks) return { line: n, written_domain: written, block, status: "no_action", issue: [] };
      const kinds = kindsOf(block);
      return kinds.length === 0
        ? { line: n, written_domain: written, block, status: "already_applied", issue: [] }
        : { line: n, written_domain: written, block, status: "importable", issue: kinds };
    });
  };

  // ---- The allowlist ---------------------------------------------------------------------------------------------

  const allows: Allow[] = empty
    ? []
    : [
        { domain: "friend.example", comment: "Our sister bakery's server.", created_at: iso(ago(30 * 24 * 60)) },
        { domain: "photos.example", comment: "", created_at: iso(ago(12 * 24 * 60)) },
      ];

  // ---- Block-list subscriptions ----------------------------------------------------------------------------------

  const fetchOf = (minutesAgo: number, n: number, more: Partial<Fetch>): Fetch => ({
    id: idAt(ago(minutesAgo), 46_600 + n),
    outcome: "applied",
    entry_count: 42,
    fetched_at: iso(ago(minutesAgo)),
    ...more,
  });
  const subscriptionOf = (key: string, n: number, s: Omit<Subscription, "id">): Subscription => {
    const id = key === "shared" ? idAt(ago(40 * 24 * 60), 46_500) : idAt(ago(10 * 24 * 60), 46_500 + n);
    keyed.set(`list:${key}`, id);
    return { id, ...s };
  };
  const sharedFetches: [Fetch, ...Fetch[]] = [
    fetchOf(60, 1, { http_status: 200 }),
    fetchOf(24 * 60 + 60, 2, { outcome: "not_modified", http_status: 304 }),
    fetchOf(2 * 24 * 60 + 60, 3, { http_status: 200, entry_count: 40 }),
  ];
  const failingFetches: [Fetch, ...Fetch[]] = [
    fetchOf(120, 4, { outcome: "failed", http_status: 404, error_code: "http_status", entry_count: 12 }),
    fetchOf(24 * 60 + 120, 5, { outcome: "failed", error_code: "network", entry_count: 12 }),
    fetchOf(2 * 24 * 60 + 120, 6, { outcome: "failed", error_code: "network", entry_count: 12 }),
    fetchOf(3 * 24 * 60 + 120, 7, { outcome: "applied", http_status: 200, entry_count: 12 }),
  ];
  const subscriptions: Subscription[] = empty
    ? []
    : [
        subscriptionOf("failing", 1, {
          url: "https://moderation.example/api/v1/instance/domain_blocks",
          format: "mastodon_json",
          title: "Neighbours' published blocks",
          mode: "automatic",
          suspend_as: "suspend",
          silence_as: "limit",
          apply_reject_media: true,
          apply_reject_reports: true,
          public_comment_from_list: true,
          consecutive_failures: 3,
          last_fetch: failingFetches[0],
          created_at: iso(ago(10 * 24 * 60)),
          updated_at: iso(ago(10 * 24 * 60)),
        }),
        subscriptionOf("shared", 0, {
          url: "https://lists.example/blocklist.csv",
          format: "mastodon_csv",
          title: "Shared block list",
          mode: "draft",
          suspend_as: "suspend",
          silence_as: "limit",
          apply_reject_media: true,
          apply_reject_reports: true,
          public_comment_from_list: false,
          consecutive_failures: 0,
          last_fetch: sharedFetches[0],
          created_at: iso(ago(40 * 24 * 60)),
          updated_at: iso(ago(40 * 24 * 60)),
        }),
      ];
  const fetches = new Map<string, Fetch[]>(
    empty
      ? []
      : [
          [keyed.get("list:shared") ?? "", sharedFetches],
          [keyed.get("list:failing") ?? "", failingFetches],
        ],
  );
  const drafts = new Map<string, Draft[]>(
    empty
      ? []
      : [
          [
            keyed.get("list:shared") ?? "",
            [
              {
                key: "lift:reject_media:mediahog.example",
                change: "lift",
                domain: "mediahog.example",
                kind: "reject_media",
                held: "draft_mode",
                action_id: keyed.get("action:media") ?? "",
              },
              {
                key: "issue:suspend:bad.example",
                change: "issue",
                domain: "bad.example",
                kind: "suspend",
                held: "draft_mode",
                severity: "suspend",
                public_comment: "Harassment campaigns.",
                obfuscate: false,
              },
              {
                key: "issue:limit:loud.example",
                change: "issue",
                domain: "loud.example",
                kind: "limit",
                held: "draft_mode",
                severity: "silence",
                obfuscate: false,
              },
            ],
          ],
          [keyed.get("list:failing") ?? "", []],
        ],
  );
  const subscriptionFound = (id: string) => subscriptions.find((s) => s.id === id);

  // ---- Remote servers --------------------------------------------------------------------------------------------

  // In domain order, as the server lists them; first seen days to months ago, but quiet.example, known only from
  // deliveries.
  const seen = (days: number) => iso(ago(days * 24 * 60));
  const servers: Server[] = empty
    ? []
    : [
        {
          domain: "a-rather-long-subdomain.of-a-community-run-server.social.example",
          state: "available",
          failure_days: 0,
          first_seen_at: seen(3),
          software: { name: "snac", version: "2.63" },
        },
        {
          domain: "dead.example",
          state: "unavailable",
          state_changed_at: iso(ago(2 * 24 * 60)),
          failure_days: 9,
          last_failure_day: iso(ago(60)).slice(0, 10),
          first_seen_at: seen(140),
          software: { name: "mastodon", version: "4.2.10" },
        },
        {
          domain: "flaky.example",
          state: "available",
          state_changed_at: iso(ago(4 * 24 * 60)),
          failure_days: 2,
          last_failure_day: iso(ago(24 * 60)).slice(0, 10),
          first_seen_at: seen(60),
          software: { name: "misskey", version: "2024.11.0" },
        },
        {
          domain: "gone.example",
          state: "unavailable",
          state_changed_at: iso(ago(30 * 24 * 60)),
          failure_days: 21,
          last_failure_day: iso(ago(3 * 60)).slice(0, 10),
          first_seen_at: seen(200),
        },
        {
          domain: "mastodon.example",
          state: "available",
          failure_days: 0,
          first_seen_at: seen(210),
          software: { name: "mastodon", version: "4.3.2" },
        },
        {
          domain: "misskey.example",
          state: "available",
          failure_days: 0,
          first_seen_at: seen(95),
          software: { name: "misskey", version: "2025.1.0" },
        },
        {
          domain: "photos.example",
          state: "available",
          failure_days: 0,
          first_seen_at: seen(45),
          software: { name: "pixelfed", version: "0.12.4" },
        },
        { domain: "quiet.example", state: "available", failure_days: 0 },
        {
          domain: "social.example",
          state: "available",
          failure_days: 0,
          first_seen_at: seen(12),
          software: { name: "gotosocial", version: "" },
        },
      ];
  const serverOf = (domain: string): Server =>
    servers.find((s) => s.domain === domain) ?? { domain, state: "available", failure_days: 0 };
  const eventsOf = (domain: string): Schema<"AdminAvailabilityEvent">[] =>
    domain === "dead.example" && !empty
      ? [
          {
            id: idAt(ago(2 * 24 * 60), 46_700),
            kind: "marked_unavailable",
            reason: "failure_days",
            occurred_at: iso(ago(2 * 24 * 60)),
          },
          {
            id: idAt(ago(20 * 24 * 60), 46_701),
            kind: "marked_available",
            reason: "inbound_request",
            occurred_at: iso(ago(20 * 24 * 60)),
          },
          {
            id: idAt(ago(28 * 24 * 60), 46_702),
            kind: "marked_unavailable",
            reason: "failure_days",
            occurred_at: iso(ago(28 * 24 * 60)),
          },
        ]
      : [];
  /** The deliveries to `domain`, newest first: a post that reached it, a masked follow, and one that keeps failing. */
  const deliveriesOf = (domain: string): Delivery[] => {
    if (empty) return [];
    const sender = c.accountOf(c.viewer);
    const failing = domain === "dead.example";
    return [
      {
        id: idAt(ago(15), 46_800),
        domain,
        activity_type: "Create",
        activity_created_at: iso(ago(16)),
        created_at: iso(ago(15)),
        shared_inbox: true,
        sender: { account_id: sender.id, username: sender.username },
        activity_uri: `https://${c.host}/users/${sender.username}/statuses/1/activity`,
        inbox_url: `https://${domain}/inbox`,
        attempts: failing
          ? [
              {
                id: idAt(ago(15), 46_810),
                outcome: "retryable_failure",
                error: "dial tcp: i/o timeout",
                attempted_at: iso(ago(15)),
              },
              {
                id: idAt(ago(5), 46_811),
                outcome: "retryable_failure",
                status_code: 503,
                signature_scheme: "rfc9421",
                attempted_at: iso(ago(5)),
              },
            ]
          : [
              {
                id: idAt(ago(15), 46_812),
                outcome: "succeeded",
                status_code: 202,
                signature_scheme: "rfc9421",
                attempted_at: iso(ago(15)),
              },
            ],
      },
      {
        id: idAt(ago(45), 46_801),
        domain,
        activity_type: "Accept",
        activity_created_at: iso(ago(46)),
        created_at: iso(ago(45)),
        shared_inbox: false,
        mask: "approves_followers",
        attempts: [
          {
            id: idAt(ago(45), 46_813),
            outcome: "succeeded",
            status_code: 202,
            signature_scheme: "cavage",
            attempted_at: iso(ago(45)),
          },
        ],
      },
    ];
  };

  // ---- Handlers ----------------------------------------------------------------------------------------------------

  const domains = (): Answer<"GET /admin/domain_actions"> | undefined =>
    may("manage_domains") ? undefined : forbidden("manage_domains");
  const handlers: Handlers = {
    "GET /admin/domain_actions": ({ query }) => {
      const refused = domains();
      if (refused !== undefined) return refused;
      const domain = query.get("domain");
      if (domain !== null && domainOf(domain) === undefined)
        return problem(400, "Bad Request", { detail: "domain is not a domain.", field: "domain" });
      const kept = actions.filter(
        (a) => (domain === null || a.domain === domainOf(domain)) && (query.get("active") !== "true" || active(a)),
      );
      return { status: 200, body: page(kept, query).body };
    },
    "POST /admin/domain_actions": ({ body }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const domain = domainOf(body.domain);
      if (domain === undefined) return badDomain();
      return { status: 201, body: structuredClone(issue(body, domain)) };
    },
    "GET /admin/domain_actions/impact": ({ query }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const domain = domainOf(query.get("domain") ?? "");
      if (domain === undefined) return badDomain();
      return { status: 200, body: { domain, ...impactOf(domain) } };
    },
    "GET /admin/domain_actions/{id}": ({ params }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const a = actions.find((x) => x.id === params.id);
      return a === undefined ? notFound() : { status: 200, body: structuredClone(a) };
    },
    "POST /admin/domain_actions/{id}/lift": ({ params }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const a = actions.find((x) => x.id === params.id);
      if (a === undefined) return notFound();
      if (active(a)) {
        a.state = "lifted";
        a.lifted_at = iso(Date.now());
        record("domain_action_lifted", a.domain, a.domain);
      }
      return { status: 200, body: structuredClone(a) };
    },
    "GET /admin/domain_actions/export": () => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const rows = [...new Set(actions.filter(active).map((a) => a.domain))].toSorted().flatMap((domain) => {
        const of = actions.filter((a) => active(a) && a.domain === domain);
        const has = (k: Action["kind"]) => of.some((a) => a.kind === k);
        if (of.every((a) => a.kind === "force_sensitive")) return [];
        const severity = has("suspend") ? "suspend" : has("limit") ? "silence" : "noop";
        const comment = of.find((a) => a.public_comment !== undefined)?.public_comment ?? "";
        return [
          [domain, severity, has("reject_media"), has("reject_reports"), comment, of.some((a) => a.obfuscate)].join(
            ",",
          ),
        ];
      });
      return {
        status: 200,
        type: "text/csv",
        raw: ["#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate", ...rows].join("\n"),
        headers: { "Content-Disposition": 'attachment; filename="domain_blocks.csv"' },
      };
    },
    "POST /admin/domain_actions/import/preview": ({ body }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const rows = preview(body.csv);
      if (rows.length > 5000)
        return problem(422, "Unprocessable Content", { detail: "More than 5,000 rows.", field: "csv" });
      return { status: 200, body: { rows } };
    },
    "POST /admin/domain_actions/import": ({ body }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const issued: Action[] = [];
      const skipped: Schema<"DomainBlockImportSkipped">[] = [];
      for (const b of body.blocks) {
        const domain = domainOf(b.domain);
        if (domain === undefined) {
          skipped.push({ domain: b.domain, reason: "invalid" });
          continue;
        }
        const kinds = kindsOf({ ...b, domain });
        if (kinds.length === 0) skipped.push({ domain: b.domain, reason: "already_applied" });
        for (const kind of kinds)
          issued.push(
            issue(
              {
                domain,
                kind,
                obfuscate: b.obfuscate,
                ...(b.public_comment === undefined ? {} : { public_comment: b.public_comment }),
                ...(body.private_note === undefined ? {} : { private_note: body.private_note }),
              },
              domain,
            ),
          );
      }
      return { status: 200, body: { issued: structuredClone(issued), skipped } };
    },
    "GET /admin/domain_allows": () => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      return { status: 200, body: { items: allows.toSorted((a, b) => a.domain.localeCompare(b.domain)) } };
    },
    "POST /admin/domain_allows": ({ body }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const domain = domainOf(body.domain);
      if (domain === undefined) return problem(400, "Bad Request", { detail: "The domain is not a host." });
      const there = allows.find((a) => a.domain === domain);
      if (there !== undefined) return { status: 200, body: there };
      const allow: Allow = { domain, comment: body.comment ?? "", created_at: iso(Date.now()) };
      allows.push(allow);
      record("domain_allow_added", domain, domain);
      return { status: 201, body: allow };
    },
    "DELETE /admin/domain_allows/{domain}": ({ params }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const at = allows.findIndex((a) => a.domain === params.domain);
      if (at >= 0) {
        allows.splice(at, 1);
        record("domain_allow_removed", params.domain, params.domain, false);
      }
      return { status: 204 };
    },
    "GET /admin/domain_list_subscriptions": ({ query }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      return { status: 200, body: page(subscriptions, query).body };
    },
    "POST /admin/domain_list_subscriptions": ({ body }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      if (!body.url.startsWith("https://"))
        return problem(422, "Unprocessable Content", { detail: "The URL must be https.", field: "url" });
      const now = Date.now();
      const first: Fetch = { id: newId(), outcome: "applied", http_status: 200, entry_count: 0, fetched_at: iso(now) };
      const s: Subscription = {
        id: newId(),
        url: body.url,
        format: body.format,
        title: body.title,
        mode: body.mode ?? "draft",
        suspend_as: body.suspend_as ?? "suspend",
        silence_as: body.silence_as ?? "limit",
        apply_reject_media: body.apply_reject_media ?? true,
        apply_reject_reports: body.apply_reject_reports ?? true,
        public_comment_from_list: body.public_comment_from_list ?? false,
        consecutive_failures: 0,
        last_fetch: first,
        created_at: iso(now),
        updated_at: iso(now),
      };
      subscriptions.unshift(s);
      fetches.set(s.id, [first]);
      drafts.set(s.id, []);
      record("domain_list_subscription_created", s.id, s.title);
      return { status: 201, body: structuredClone(s) };
    },
    "GET /admin/domain_list_subscriptions/{id}": ({ params }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const s = subscriptionFound(params.id);
      return s === undefined ? notFound() : { status: 200, body: structuredClone(s) };
    },
    "PATCH /admin/domain_list_subscriptions/{id}": ({ params, body }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const s = subscriptionFound(params.id);
      if (s === undefined) return notFound();
      Object.assign(s, body, { updated_at: iso(Date.now()) });
      record("domain_list_subscription_updated", s.id, s.title);
      return { status: 200, body: structuredClone(s) };
    },
    "DELETE /admin/domain_list_subscriptions/{id}": ({ params, query }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const at = subscriptions.findIndex((s) => s.id === params.id);
      const s = subscriptions[at];
      if (s === undefined) return notFound();
      subscriptions.splice(at, 1);
      if (query.get("lift_actions") === "true")
        for (const a of actions)
          if (a.subscription_id === s.id && active(a)) {
            a.state = "lifted";
            a.lifted_at = iso(Date.now());
          }
      record("domain_list_subscription_deleted", s.id, s.title, false);
      return { status: 204 };
    },
    "POST /admin/domain_list_subscriptions/{id}/fetch": ({ params }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const s = subscriptionFound(params.id);
      if (s === undefined) return notFound();
      record("domain_list_fetch_queued", s.id, s.title);
      return { status: 202 };
    },
    "GET /admin/domain_list_subscriptions/{id}/fetches": ({ params, query }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      if (subscriptionFound(params.id) === undefined) return notFound();
      return { status: 200, body: page(fetches.get(params.id) ?? [], query).body };
    },
    "GET /admin/domain_list_subscriptions/{id}/drafts": ({ params }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      if (subscriptionFound(params.id) === undefined) return notFound();
      return { status: 200, body: { items: drafts.get(params.id) ?? [] } };
    },
    "POST /admin/domain_list_subscriptions/{id}/drafts/accept": ({ params, body }) =>
      decide(params.id, body?.keys, true),
    "POST /admin/domain_list_subscriptions/{id}/drafts/reject": ({ params, body }) =>
      decide(params.id, body?.keys, false),
    "GET /admin/instances": ({ query }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const state = query.get("state");
      const troubled = query.get("troubled") === "true";
      return {
        status: 200,
        body: page(
          servers.filter(
            (s) =>
              (state === null || s.state === state) && (!troubled || s.state === "unavailable" || s.failure_days > 0),
          ),
          query,
          (s) => s.domain,
        ).body,
      };
    },
    "GET /admin/instances/{domain}": ({ params }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      return { status: 200, body: structuredClone(serverOf(params.domain)) };
    },
    "GET /admin/instances/{domain}/availability_events": ({ params, query }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      return { status: 200, body: page(eventsOf(params.domain), query).body };
    },
    "GET /admin/instances/{domain}/deliveries": ({ params, query }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const accountId = query.get("account_id");
      const activity = query.get("activity_uri");
      if (accountId !== null) {
        const account = c.accountById(accountId);
        if (account === undefined) return notFound();
        if (account.acct.includes("@"))
          return problem(400, "Bad Request", { detail: "The account is not local.", field: "account_id" });
        record("account_deliveries_viewed", accountId, `@${account.acct} ${params.domain}`);
      }
      const kept = deliveriesOf(params.domain).filter(
        (d) =>
          (activity === null || d.activity_uri === activity) &&
          (accountId === null || d.sender?.account_id === accountId || d.mask !== undefined),
      );
      return { status: 200, body: page(kept, query).body };
    },
    "POST /admin/instances/{domain}/deliveries/{id}/reveal": ({ params }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const d = deliveriesOf(params.domain).find((x) => x.id === params.id);
      if (d === undefined) return notFound();
      record("delivery_revealed", params.domain, params.domain);
      const sender = c.accountOf(c.viewer);
      return {
        status: 200,
        body: {
          id: d.id,
          ...(d.mask === undefined ? {} : { mask: d.mask }),
          sender: { account_id: sender.id, username: sender.username },
          activity_uri: d.activity_uri ?? `https://${c.host}/users/${sender.username}#accepts/follows/1`,
          inbox_url: d.inbox_url ?? `https://${params.domain}/users/someone/inbox`,
        },
      };
    },
    "POST /admin/instances/{domain}/mark_available": ({ params }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      const s = servers.find((x) => x.domain === params.domain);
      if (s !== undefined && (s.state === "unavailable" || s.failure_days > 0)) {
        s.state = "available";
        s.failure_days = 0;
        s.state_changed_at = iso(Date.now());
        delete s.last_failure_day;
        record("instance_marked_available", s.domain, s.domain);
      }
      return { status: 200, body: structuredClone(serverOf(params.domain)) };
    },
    "POST /admin/instances/{domain}/refetch_statuses": ({ params }) => {
      if (!may("manage_domains")) return forbidden("manage_domains");
      if (params.domain === c.host) return problem(422, "Unprocessable Content", { detail: "This server's domain." });
      record("domain_statuses_refetch_queued", params.domain, params.domain);
      return { status: 202 };
    },
    "GET /admin/audit_log": ({ query }) => {
      if (!may("view_audit")) return forbidden("view_audit");
      const actor = query.get("actor_account_id");
      const action = query.get("action");
      const kind = query.get("target_kind");
      const target = query.get("target_id");
      if (target !== null && kind === null)
        return problem(400, "Bad Request", { detail: "target_id needs target_kind." });
      const kept = audit.filter(
        (e) =>
          (actor === null || e.actor_account_id === actor) &&
          (action === null || e.action === action) &&
          (kind === null || e.target_kind === kind) &&
          (target === null || e.target_id === target),
      );
      return { status: 200, body: page(kept, query).body };
    },
  };

  /** Accepts or rejects the drafts of `keys` (every draft without), as the server does, each recorded. */
  function decide(
    id: string,
    keys: readonly string[] | undefined,
    accept: boolean,
  ): Answer<"POST /admin/domain_list_subscriptions/{id}/drafts/accept"> {
    if (!may("manage_domains")) return forbidden("manage_domains");
    const s = subscriptionFound(id);
    if (s === undefined) return notFound();
    const all = drafts.get(id) ?? [];
    const asked = keys ?? all.map((d) => d.key);
    const done = all.filter((d) => asked.includes(d.key));
    const missing = asked.filter((k) => !all.some((d) => d.key === k));
    drafts.set(
      id,
      all.filter((d) => !asked.includes(d.key)),
    );
    for (const d of done) {
      record(accept ? "domain_list_draft_accepted" : "domain_list_draft_rejected", s.id, s.title);
      if (!accept) continue;
      if (d.change === "issue") {
        const action = issue({ domain: d.domain, kind: d.kind, obfuscate: d.obfuscate ?? false }, d.domain);
        action.subscription_id = s.id;
      } else {
        const a = actions.find((x) => x.id === d.action_id);
        if (a !== undefined && active(a)) {
          a.state = "lifted";
          a.lifted_at = iso(Date.now());
          record("domain_action_lifted", a.domain, a.domain);
        }
      }
    }
    return { status: 200, body: { done, missing } };
  }

  return {
    handlers,
    domains: {
      id: (kind, key) => keyed.get(`${kind}:${key}`) ?? "none",
      audit: () => audit,
    },
  };
}
