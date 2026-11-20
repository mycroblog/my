// SPDX-License-Identifier: AGPL-3.0-only

// The fake's instance settings as an administrator keeps them (FR-001, FR-010, FR-705, FR-724, FR-725, FR-922,
// ADR-0058): every setting with what it accepts, as the server defines them (internal/domain/instance/setting.go),
// saved together or refused together with the key named (`values.<key>`); what switching federation to the allowlist
// would end, and who limiting sign-in to the login providers would keep out; the instance's texts; its brand images,
// an SVG checked as the server checks it (refused with text in it, its scripts removed); the translation providers the
// server has; the rules, two of them retired; the relays, one joined, one waiting, one that rejected the server; the
// login providers, one of each kind but Discord; the OAuth apps of every way of registration, one disabled; and the
// hosts denied, two of them.

import { idAt, minute, page, problem, type PageQuery } from "./common";
import { fakeRules } from "./reports";
import type { Answer, Handlers, Refusal, Schema } from "./spec";

type Setting = Schema<"AdminSetting">;
type Key = Schema<"InstanceSettingKey">;
type Kind = Schema<"InstanceSettingKind">;

/** What the administration's lists hold when the fake starts: the fake's own (by default), or none. */
export type FakeAdminOptions = {
  /** The relays, the login providers and the OAuth apps besides the server's own: the fake's, or none. */
  lists?: "fake" | "none";
  /** Limiting sign-in to the login providers is on, while none is enabled: adding or enabling one starts it. */
  providersOnlyWaiting?: boolean;
};

/** What the administration's handlers take from the fake. */
export type AdminContext = {
  readonly started: number;
  readonly options: FakeAdminOptions | undefined;
  /** The settings as the fake's instance says them, over the server's defaults. */
  readonly initial: Readonly<Partial<Record<Key, string>>>;
  /** The texts of the instance as it starts. */
  readonly texts: Readonly<Partial<Record<Schema<"InstanceTextKey">, string>>>;
  /** The brand images of the instance as it starts. */
  readonly brands: Schema<"AdminBrands">;
  /** The translation providers the server has. */
  readonly translationProviders: readonly Schema<"TranslationProvider">[];
  readonly permissions: () => readonly Schema<"Permission">[];
  readonly accountOf: (username: string) => Schema<"Account">;
  /** The username of the reader. */
  readonly viewer: string;
  readonly host: string;
};

/** A definition of the server's: its key, kind, default, and its choices or bounds. */
type Definition = readonly [Key, Kind, string, (readonly string[] | readonly [number, number])?];

const audience = ["off", "signed_in", "everyone"] as const;
const hookModes = ["off", "observe", "enforce"] as const;

/** The server's definitions, in its order (instance.Definitions). */
const definitions: readonly Definition[] = [
  ["registration_mode", "enum", "approval", ["open", "approval", "invite", "closed"]],
  ["max_status_length", "int", "500", [1, 100000]],
  ["default_locale", "locale", "en"],
  ["local_timeline_visibility", "enum", "everyone", audience],
  ["federated_timeline_visibility", "enum", "signed_in", audience],
  ["preview_image", "media", ""],
  ["contact_account", "account", ""],
  ["contact_email", "email", ""],
  ["thumbnail", "media", ""],
  ["remote_media_policy", "enum", "none", ["none", "avatars", "all"]],
  ["remote_media_cache_ttl_seconds", "int", "86400", [60, 31536000]],
  ["authorized_fetch", "bool", "false"],
  ["domain_actions_visibility", "enum", "off", audience],
  ["domain_actions_rationale_visibility", "enum", "off", audience],
  ["appeal_window_days", "int", "180", [20, 365]],
  ["session_origins_retention_days", "int", "180", [90, 3650]],
  ["registration_origins_retention_days", "int", "180", [90, 3650]],
  ["reports_retention_days", "int", "365", [180, 3650]],
  ["moderation_actions_retention_days", "int", "365", [180, 3650]],
  ["audit_log_retention_days", "int", "730", [365, 3650]],
  ["search_indexing", "enum", "deny", ["allow", "deny"]],
  ["search_visibilities", "enum", "public", ["public", "local", "public,local"]],
  ["ai_usage", "enum", "deny", ["allow", "deny_training", "deny"]],
  ["dynamic_client_registration", "bool", "true"],
  ["indexable_default", "bool", "false"],
  ["security_contact", "uri", ""],
  ["require_media_description", "bool", "false"],
  ["oauth_metadata_document_clients", "bool", "true"],
  ["link_cards", "bool", "true"],
  ["link_cards_ignore_robots_txt", "bool", "false"],
  ["ux_measurement", "bool", "false"],
  ["ux_measurement_share", "int", "10", [1, 100]],
  ["finance_visibility", "enum", "private", ["private", "totals", "breakdown"]],
  ["finance_viewer_item_names", "bool", "false"],
  ["finance_viewer_item_memos", "bool", "false"],
  ["finance_viewer_income", "enum", "totals", ["totals", "plans"]],
  ["supporter_list_visibility", "enum", "off", audience],
  ["income_visibility", "enum", "private", ["private", "totals", "plans"]],
  ["funding_meter_visibility", "enum", "off", ["off", "percent", "amounts"]],
  ["funding_meter_target_amount", "decimal", ""],
  ["funding_meter_target_currency", "currency", ""],
  ["funding_meter_rail", "bool", "false"],
  ["federation_mode", "enum", "both", ["off", "inbound", "outbound", "both"]],
  ["federation_scope", "enum", "open", ["open", "allowlist"]],
  ["user_app_registration", "bool", "true"],
  ["voice_room_starters", "enum", "permitted", ["everyone", "permitted"]],
  ["voice_public_summary", "bool", "false"],
  ["voice_participants_retention_days", "int", "90", [30, 3650]],
  ["hook_inbound_status_mode", "enum", "observe", hookModes],
  ["hook_inbound_follow_mode", "enum", "observe", hookModes],
  ["hook_signup_check_mode", "enum", "observe", hookModes],
  ["hook_media_check_mode", "enum", "observe", hookModes],
  ["hook_local_status_mode", "enum", "observe", hookModes],
  ["hook_inbound_actor_mode", "enum", "observe", hookModes],
  ["hook_local_profile_mode", "enum", "observe", hookModes],
  ["hook_inbound_status_on_failure", "enum", "pass", ["pass", "retry"]],
  ["hook_inbound_follow_on_failure", "enum", "pass", ["pass", "retry"]],
  ["hook_signup_check_on_failure", "enum", "pass", ["pass", "require_approval"]],
  ["hook_media_check_on_failure", "enum", "pass", ["pass", "retry"]],
  ["hook_inbound_actor_on_failure", "enum", "pass", ["pass", "retry"]],
  ["hook_inbound_status_unfollowed_direct", "bool", "false"],
  ["hook_signup_check_reject", "bool", "false"],
  ["hook_local_status_account_age_days", "int", "7", [0, 3650]],
  ["hook_local_profile_account_age_days", "int", "7", [0, 3650]],
  ["hook_decisions_retention_days", "int", "90", [30, 3650]],
  ["event_creators", "enum", "everyone", ["everyone", "permitted"]],
  ["event_records_retention_days", "int", "90", [30, 3650]],
  ["invite_creators", "enum", "permitted", ["everyone", "permitted"]],
  ["member_invite_max_active", "int", "5", [0, 100]],
  ["member_invite_max_uses", "int", "10", [1, 1000]],
  ["member_invite_max_lifetime_seconds", "int", "2592000", [3600, 31536000]],
  ["public_counts", "bool", "true"],
  ["featured_accounts", "accounts", ""],
  ["remote_media_cache_max_bytes", "int", "1073741824", [1, 1125899906842624]],
  ["remote_status_retention_days", "int", "90", [1, 3650]],
  ["remote_status_max_retention_days", "int", "0", [0, 3650]],
  ["relay_status_retention_days", "int", "7", [1, 3650]],
  ["oauth_client_rate_limits", "client_limits", ""],
  ["login_providers_only", "bool", "false"],
  ["translation_providers", "provider_list", ""],
  ["translation_provider_visibilities", "provider_visibilities", ""],
  ["translation_browser", "bool", "false"],
  ["translation_user_monthly_characters", "int", "50000", [0, 1000000000000]],
  ["translation_server_monthly_characters", "int", "500000", [0, 1000000000000]],
  ["translation_retention_days", "int", "30", [1, 3650]],
  ["translation_reports_to_server", "bool", "false"],
  ["auto_delete_default_enabled", "bool", "false"],
  ["auto_delete_default_min_age_days", "int", "30", [1, 3650]],
  ["auto_delete_default_keep_pinned", "bool", "true"],
  ["auto_delete_default_keep_bookmarked", "bool", "true"],
  ["auto_delete_default_keep_favourited", "bool", "true"],
  ["auto_delete_default_keep_media", "bool", "false"],
  ["auto_delete_default_keep_polls", "bool", "false"],
  ["auto_delete_default_keep_articles", "bool", "true"],
  ["auto_delete_default_keep_public", "bool", "false"],
  ["auto_delete_default_keep_local", "bool", "false"],
  ["auto_delete_default_keep_unlisted", "bool", "false"],
  ["auto_delete_default_keep_private", "bool", "false"],
  ["auto_delete_default_keep_direct", "bool", "true"],
  ["auto_delete_default_keep_min_boosts", "int", "0", [0, 1000000]],
  ["auto_delete_default_keep_min_favourites", "int", "0", [0, 1000000]],
  ["auto_delete_max_age_days", "int", "0", [0, 3650]],
  ["auto_delete_max_allow_pinned", "bool", "true"],
  ["auto_delete_max_kept_pins", "int", "5", [1, 5]],
  ["auto_delete_max_allow_bookmarked", "bool", "false"],
  ["auto_delete_max_allow_favourited", "bool", "false"],
  ["auto_delete_max_allow_media", "bool", "false"],
  ["auto_delete_max_allow_polls", "bool", "false"],
  ["auto_delete_max_allow_articles", "bool", "true"],
  ["auto_delete_max_allow_visibilities", "bool", "false"],
  ["auto_delete_max_allow_thresholds", "bool", "false"],
  ["auto_delete_grace_days", "int", "30", [0, 365]],
  ["single_user", "bool", "false"],
  ["max_drafts", "int", "100", [1, 10000]],
  ["max_scheduled_posts", "int", "300", [1, 10000]],
  ["max_scheduled_posts_per_day", "int", "25", [1, 10000]],
  ["scheduled_post_min_lead_minutes", "int", "5", [1, 1440]],
  ["scheduled_post_max_lead_days", "int", "366", [1, 3660]],
  ["article_authoring", "enum", "everyone", ["everyone", "permission"]],
  ["max_article_bytes", "int", "100000", [1000, 100000]],
  ["max_article_images", "int", "20", [0, 100]],
];

/** Whether `value` is one `kind` takes, as far as the fake checks: a number in its bounds, a choice, an address. */
function accepts(d: Definition, value: string): boolean {
  const [, kind, , more] = d;
  if (kind === "bool") return value === "true" || value === "false";
  if (kind === "int") {
    const [min, max] = (more ?? [0, 0]) as readonly [number, number];
    return /^\d+$/.test(value) && Number(value) >= min && Number(value) <= max;
  }
  if (kind === "enum") return (more as readonly string[]).includes(value);
  if (kind === "email") return value === "" || /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(value);
  if (kind === "uri") return value === "" || /^(https:\/\/|mailto:)\S+$/.test(value);
  return true;
}

/** The handlers of the administration's settings, texts, rules, relays, login providers and OAuth apps. */
/** What a test or a story does with the administration's fake: the IDs of what its lists hold as it starts. */
export type FakeAdmin = {
  /** The ID of the rule at `index` of the rules as the fake starts, those in effect first, then the retired ones. */
  readonly ruleId: (index: number) => string;
  /** The ID of the relay at `index` of the fake's (joined, waiting, rejected), or "none". */
  readonly relayId: (index: number) => string;
  /** The ID of the login provider at `index` of the fake's, newest first, or "none". */
  readonly providerId: (index: number) => string;
  /** The value of the setting of `key` now, saved or the server's default. */
  readonly setting: (key: Key) => string;
};

export function adminHandlers(c: AdminContext): { handlers: Handlers; admin: FakeAdmin } {
  const iso = (ms: number) => new Date(ms).toISOString();
  const ago = (minutes: number) => c.started - minutes * minute;
  const none = c.options?.lists === "none";
  const allowed = (p: Schema<"Permission">) => c.permissions().includes(p);
  const forbidden = () => problem(403, "Forbidden", { detail: "Your account does not have the permission." });
  /** A problem with a member of its own (`reason`, `brand_error`), as the server's typed problems carry. */
  const typed = (title: string, type: string, more: Readonly<Record<string, unknown>>): Refusal => ({
    problem: { type, title, status: 422, ...more },
  });

  // ---- The settings -----------------------------------------------------------------------------------------------

  const stored = new Map<Key, string>(Object.entries(c.initial).map(([k, v]) => [k as Key, v] as const));
  if (c.options?.providersOnlyWaiting === true) stored.set("login_providers_only", "true");
  const thumbnail = `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 400"><rect width="1200" height="400" fill="#d6e6df"/><path d="M0 290 Q300 200 620 270 T1200 230 V400 H0Z" fill="#86b8a2"/></svg>`,
  )}`;
  const settingOf = (d: Definition): Setting => {
    const [key, kind, def, more] = d;
    const value = stored.get(key) ?? def;
    return {
      key,
      kind,
      value,
      default: def,
      stored: stored.has(key),
      ...(kind === "enum" ? { choices: [...(more as readonly string[])] } : {}),
      ...(kind === "int"
        ? { minimum: (more as readonly [number, number])[0], maximum: (more as readonly [number, number])[1] }
        : {}),
      ...(kind === "media" && value !== "" ? { image: { url: thumbnail, width: 1200, height: 400 } } : {}),
    };
  };
  const list = (): Schema<"AdminSettingList"> => ({ items: definitions.map(settingOf) });
  const valueOf = (key: Key) => stored.get(key) ?? definitions.find(([k]) => k === key)?.[2] ?? "";

  // ---- The texts --------------------------------------------------------------------------------------------------

  const texts = new Map(Object.entries(c.texts).flatMap(([k, v]) => (v === "" ? [] : [[k, v] as const])));
  const textLimits: Readonly<Record<string, number>> = {
    name: 100,
    short_description: 500,
    description: 20_000,
    preview_image_alt: 1500,
    thumbnail_alt: 1500,
  };
  const revised = iso(ago(3 * 24 * 60));

  // ---- The brand images -------------------------------------------------------------------------------------------

  const brands = new Map<Schema<"BrandKind">, Schema<"AdminBrandImage">>(
    (["icon", "logo", "logo_dark"] as const).flatMap((k) => {
      const image = c.brands[k];
      return image === undefined ? [] : [[k, image] as const];
    }),
  );
  const sizes: Readonly<Record<Schema<"BrandKind">, Schema<"BrandRenditionSize">[]>> = {
    icon: [
      { rendition: "favicon-32", width: 32, height: 32 },
      { rendition: "favicon-48", width: 48, height: 48 },
      { rendition: "apple-touch-180", width: 180, height: 180 },
      { rendition: "pwa-192", width: 192, height: 192 },
      { rendition: "pwa-512", width: 512, height: 512 },
      { rendition: "maskable-512", width: 512, height: 512 },
    ],
    logo: [
      { rendition: "logo-mail", width: 384, height: 96 },
      { rendition: "preview-1200x630", width: 1200, height: 630 },
    ],
    logo_dark: [{ rendition: "logo-dark-mail", width: 384, height: 96 }],
  };
  const brandRefused = (code: Schema<"BrandError">["code"]): Refusal =>
    typed("The image cannot be used", "/api/v1/problems/invalid-brand", { brand_error: { code } });
  /** The SVG as the server would keep it: refused with text in it or no viewBox, its scripts removed. */
  async function sanitized(request: Request, kind: Schema<"BrandKind">): Promise<Schema<"BrandSanitized"> | Refusal> {
    const form = await request.formData();
    const file = form.get("svg");
    const text = file instanceof Blob ? await file.text() : "";
    if (!text.includes("<svg")) return brandRefused("not_svg");
    if (/<text[\s>]/.test(text)) return brandRefused("text");
    const box = /viewBox="([\d.\s-]+)"/.exec(text)?.[1]?.trim().split(/\s+/).map(Number);
    if (box === undefined || box.length !== 4) return brandRefused("no_viewbox");
    const [, , w = 0, h = 0] = box;
    if (kind === "icon" && Math.abs(w - h) > w / 100) return brandRefused("aspect_ratio");
    const scripts = /<script[\s\S]*?<\/script>/g;
    const dropped: Schema<"BrandDropped">[] = scripts.test(text) ? [{ kind: "element", name: "script" }] : [];
    return {
      // As the server's sanitizer writes it: the viewBox's size as the root's own width and height.
      svg: text
        .replace(scripts, "")
        .replace(/<svg\b[^>]*>/, (open) =>
          open.replace(/\s(?:width|height)="[^"]*"/g, "").replace(/>$/, ` width="${String(w)}" height="${String(h)}">`),
        ),
      view_box_width: w,
      view_box_height: h,
      renditions: sizes[kind],
      dropped,
    };
  }

  // ---- The rules --------------------------------------------------------------------------------------------------

  const { rules: inEffect, retired } = fakeRules(c.started);
  const created = ago(400 * 24 * 60);
  let rules: Schema<"AdminRule">[] = [
    ...inEffect.map((r, i) => ({
      ...r,
      position: i + 1,
      created_at: iso(created + i * minute),
      revised_at: iso(created + i * minute),
    })),
    {
      id: retired,
      position: 5,
      text: "No posts about other servers' drama.",
      created_at: iso(created + 5 * minute),
      revised_at: iso(created + 5 * minute),
      retired_at: iso(ago(30 * 24 * 60)),
    },
    {
      id: idAt(created, 40_006),
      position: 6,
      text: "Use content warnings for food.",
      created_at: iso(created + 6 * minute),
      revised_at: iso(created + 6 * minute),
      retired_at: iso(ago(90 * 24 * 60)),
    },
  ];
  let ruleIds = 10;
  const inForce = () => rules.filter((r) => r.retired_at === undefined).toSorted((a, b) => a.position - b.position);
  const renumber = (order: readonly Schema<"AdminRule">[]) => {
    const places = new Map(order.map((r, i) => [r.id, i + 1]));
    rules = rules.map((r) => ({ ...r, position: places.get(r.id) ?? r.position }));
  };
  const ruleRefusal = (text: string, hint: string | undefined): Refusal | undefined => {
    if (text.trim() === "" || text.length > 300 || text.includes("\n"))
      return problem(422, "Unprocessable Content", { detail: "This value is not valid.", field: "text" });
    if (hint !== undefined && hint.length > 1000)
      return problem(422, "Unprocessable Content", { detail: "This value is not valid.", field: "hint" });
    return undefined;
  };

  // ---- The relays -------------------------------------------------------------------------------------------------

  const relay = (
    n: number,
    url: string,
    state: Schema<"RelayState">,
    more: Partial<Schema<"AdminRelay">> = {},
  ): Schema<"AdminRelay"> => ({
    id: idAt(ago(60 * 24 * 60 - n), 47_000 + n),
    protocol: "mastodon",
    url,
    inbox_url: url,
    receive: true,
    state,
    state_changed_at: iso(ago(20 * 24 * 60 - n * 600)),
    taken_last_day: 0,
    dropped_last_day: 0,
    domain_suspended: false,
    created_at: iso(ago(60 * 24 * 60 - n)),
    ...more,
  });
  let relays: Schema<"AdminRelay">[] = none
    ? []
    : [
        relay(1, "https://relay.fedi.example/inbox", "accepted", {
          actor_uri: "https://relay.fedi.example/actor",
          taken_last_day: 1284,
          dropped_last_day: 12,
        }),
        relay(2, "https://relay.small.example/actor", "pending", {
          protocol: "litepub",
          inbox_url: "https://relay.small.example/inbox",
          receive: false,
        }),
        relay(3, "https://relay.closed.example/inbox", "rejected"),
      ];

  // ---- The login providers ----------------------------------------------------------------------------------------

  const provider = (
    n: number,
    kind: Schema<"LoginProviderKind">,
    name: string,
    more: Partial<Schema<"AdminLoginProvider">> = {},
  ): Schema<"AdminLoginProvider"> => {
    const id = idAt(ago(30 * 24 * 60 - n), 48_000 + n);
    return {
      id,
      kind,
      display_name: name,
      client_id: `client-${String(n)}-4f9a2c`,
      scopes: [],
      allow_signup: false,
      allow_link: true,
      claims: [],
      organizations: [],
      guilds: [],
      state: "enabled",
      state_changed_at: iso(ago(30 * 24 * 60 - n)),
      callback_url: `https://${c.host}/auth/external/${id}/callback`,
      linked_users: 0,
      created_at: iso(ago(30 * 24 * 60 - n)),
      revised_at: iso(ago(30 * 24 * 60 - n)),
      ...more,
    };
  };
  let providers: Schema<"AdminLoginProvider">[] =
    none || c.options?.providersOnlyWaiting === true
      ? []
      : [
          provider(3, "mastodon", "mastodon.example", {
            issuer: "https://mastodon.example",
            state: "disabled",
            linked_users: 2,
          }),
          provider(2, "github", "GitHub", { organizations: ["river-park"], linked_users: 5, allow_signup: true }),
          provider(1, "oidc", "Town Workspace", {
            issuer: "https://accounts.town.example",
            claims: [{ claim: "hd", values: ["town.example"] }],
            allow_signup: true,
            linked_users: 9,
          }),
        ];
  let providerIds = 10;
  const providerRefused = (reason: Schema<"LoginProviderProblemReason">): Refusal =>
    typed("The login provider cannot be set up", "/api/v1/problems/invalid-login-provider", { reason });
  const enabledProviders = () => providers.filter((p) => p.state === "enabled").length;

  // ---- The OAuth apps and the denied hosts -----------------------------------------------------------------------

  const app = (
    n: number,
    name: string,
    registration: Schema<"OAuthClientRegistration">,
    more: Partial<Schema<"AdminOAuthClient">> = {},
  ): Schema<"AdminOAuthClient"> => ({
    id: idAt(ago(n * 24 * 60), 49_000 + n),
    client_id: `${registration}-${String(n)}`,
    name,
    registration,
    confidential: false,
    scopes: ["read", "write"],
    owner_deleted: false,
    state: "enabled",
    created_at: iso(ago(n * 24 * 60)),
    ...more,
  });
  let apps: Schema<"AdminOAuthClient">[] = [
    app(1, "Pocket Reader", "metadata_document", { client_id: "https://pocket.example/client.json" }),
    ...(none
      ? []
      : [
          app(2, "mycroblog CLI helper", "dynamic"),
          app(3, "River bot", "user", { owner: c.accountOf(c.viewer), confidential: true }),
          app(4, "Spammy poster", "user", { owner_deleted: true, state: "disabled" }),
          app(5, "Feed to mail", "metadata_document", { client_id: "https://feeds.example/app.json" }),
        ]),
    app(400, "mycroblog", "first_party", { client_id: "web", scopes: ["read", "write", "follow", "push"] }),
  ];
  let denials: Schema<"AdminOAuthClientHostDenial">[] = none
    ? []
    : [
        {
          id: idAt(ago(5 * 24 * 60), 49_500),
          host: "clients.spam.example",
          denied_by: c.accountOf(c.viewer),
          denied_by_deleted: false,
          denied_at: iso(ago(5 * 24 * 60)),
        },
        {
          id: idAt(ago(12 * 24 * 60), 49_501),
          host: "apps.junk.example",
          denied_by: c.accountOf(c.viewer),
          denied_by_deleted: false,
          denied_at: iso(ago(12 * 24 * 60)),
        },
      ];

  const pageOf = <T extends { id: string }>(items: readonly T[], query: PageQuery) => page(items, query).body;

  const handlers: Handlers = {
    "GET /admin/settings": () => (allowed("manage_settings") ? { status: 200, body: list() } : forbidden()),
    "PATCH /admin/settings": ({ body }) => {
      if (!allowed("manage_settings")) return forbidden();
      const invalid = (key: string) =>
        problem(422, "Unprocessable Content", { detail: "This value is not valid.", field: `values.${key}` });
      for (const [key, value] of Object.entries(body.values)) {
        const d = definitions.find(([k]) => k === key);
        if (d === undefined || !accepts(d, value)) return invalid(key);
        if (key.startsWith("hook_") && key.endsWith("_mode") && value === "enforce" && valueOf(d[0]) === "off")
          return invalid(key);
      }
      for (const [key, value] of Object.entries(body.values)) stored.set(key as Key, value);
      return { status: 200, body: list() };
    },
    "GET /admin/settings/federation_scope/impact": () =>
      allowed("manage_settings")
        ? {
            status: 200,
            body: {
              scope: valueOf("federation_scope") === "allowlist" ? "allowlist" : "open",
              follows: 37,
              local_accounts: 12,
              domains: 5,
              top_domains: [
                { domain: "mastodon.example", follows: 18 },
                { domain: "photos.example", follows: 9 },
                { domain: "misskey.example", follows: 6 },
              ],
            },
          }
        : forbidden(),
    "GET /admin/settings/login_providers_only/impact": ({ query }) => {
      if (!allowed("manage_settings") && !allowed("manage_login_providers")) return forbidden();
      const id = query.get("provider_id");
      if (id !== null && !providers.some((p) => p.id === id)) return problem(404, "Not Found");
      const on = valueOf("login_providers_only") === "true";
      const enabled = enabledProviders();
      return {
        status: 200,
        body: {
          setting_on: on,
          enabled_providers: enabled,
          users_without_provider: 14,
          includes_viewer: true,
          starts_limit: on && enabled === 0,
        },
      };
    },
    "GET /admin/instance_texts": () =>
      allowed("manage_settings")
        ? {
            status: 200,
            body: {
              items: [...texts].map(([key, body]) => ({
                key: key as Schema<"InstanceTextKey">,
                body,
                revised_at: revised,
              })),
            },
          }
        : forbidden(),
    "PUT /admin/instance_texts/{key}": ({ params, body }) => {
      if (!allowed("manage_settings")) return forbidden();
      const limit = textLimits[params.key] ?? 100_000;
      if (body.body.length > limit || (params.key === "name" && body.body.includes("\n")))
        return problem(422, "Unprocessable Content", { detail: "This value is not valid.", field: "body" });
      if (body.body === "") texts.delete(params.key);
      else texts.set(params.key, body.body);
      return { status: 200, body: { key: params.key, body: body.body, revised_at: iso(Date.now()) } };
    },
    "GET /admin/instance/brand": () =>
      allowed("manage_settings") ? { status: 200, body: Object.fromEntries(brands) } : forbidden(),
    "POST /admin/instance/brand/sanitize": async ({ request }) => {
      if (!allowed("manage_settings")) return forbidden();
      const form = await request.clone().formData();
      const kind = form.get("kind");
      const answer = await sanitized(request, kind === "logo" || kind === "logo_dark" ? kind : "icon");
      return "problem" in answer ? answer : { status: 200, body: answer };
    },
    "PUT /admin/instance/brand/{kind}": async ({ params, request }) => {
      if (!allowed("manage_settings")) return forbidden();
      const answer = await sanitized(request.clone(), params.kind);
      if ("problem" in answer) return answer;
      const form = await request.formData();
      const background = form.get("background");
      const image: Schema<"AdminBrandImage"> = {
        revision: String(Date.now()),
        svg_url: `data:image/svg+xml,${encodeURIComponent(answer.svg)}`,
        width: Math.round(answer.view_box_width),
        height: Math.round(answer.view_box_height),
        renditions: [],
        ...(params.kind === "icon" && typeof background === "string" ? { background } : {}),
        revised_at: iso(Date.now()),
        dropped: answer.dropped,
      };
      brands.set(params.kind, image);
      return { status: 200, body: image };
    },
    "DELETE /admin/instance/brand/{kind}": ({ params }) => {
      if (!allowed("manage_settings")) return forbidden();
      brands.delete(params.kind);
      return { status: 204 };
    },
    "GET /admin/translation": () =>
      allowed("manage_settings")
        ? {
            status: 200,
            body: {
              month: new Date(c.started).toISOString().slice(0, 7),
              characters: 128_400,
              cache_hits: 310,
              cache_misses: 122,
              cache_hit_rate: 310 / 432,
              providers: [...c.translationProviders],
            },
          }
        : forbidden(),

    "GET /admin/rules": ({ query }) => {
      if (!allowed("manage_rules")) return forbidden();
      const retiredToo = query.get("include_retired") === "true";
      return {
        status: 200,
        body: {
          items: [...inForce(), ...(retiredToo ? rules.filter((r) => r.retired_at !== undefined) : [])],
        },
      };
    },
    "POST /admin/rules": ({ body }) => {
      if (!allowed("manage_rules")) return forbidden();
      const refused = ruleRefusal(body.text, body.hint);
      if (refused !== undefined) return refused;
      if (inForce().length >= 100)
        return problem(422, "Unprocessable Content", { detail: "There are 100 rules.", field: "rules" });
      ruleIds += 1;
      const now = iso(Date.now());
      const rule: Schema<"AdminRule"> = {
        id: idAt(Date.now(), 40_000 + ruleIds),
        position: inForce().length + 1,
        text: body.text,
        ...(body.hint === undefined || body.hint === "" ? {} : { hint: body.hint }),
        created_at: now,
        revised_at: now,
      };
      rules = [...rules, rule];
      return { status: 201, body: rule };
    },
    "GET /admin/rules/{id}": ({ params }) => {
      const rule = rules.find((r) => r.id === params.id);
      return rule === undefined ? problem(404, "Not Found") : { status: 200, body: rule };
    },
    "PUT /admin/rules/{id}": ({ params, body }) => {
      const rule = rules.find((r) => r.id === params.id);
      if (rule === undefined) return problem(404, "Not Found");
      if (rule.retired_at !== undefined) return problem(409, "Conflict");
      const refused = ruleRefusal(body.text, body.hint);
      if (refused !== undefined) return refused;
      const next: Schema<"AdminRule"> = {
        id: rule.id,
        position: rule.position,
        text: body.text,
        ...(body.hint === undefined || body.hint === "" ? {} : { hint: body.hint }),
        created_at: rule.created_at,
        revised_at: iso(Date.now()),
      };
      rules = rules.map((r) => (r.id === rule.id ? next : r));
      return { status: 200, body: next };
    },
    "POST /admin/rules/{id}/move": ({ params, body }) => {
      const rule = rules.find((r) => r.id === params.id);
      if (rule === undefined) return problem(404, "Not Found");
      if (rule.retired_at !== undefined) return problem(409, "Conflict");
      const order = inForce().filter((r) => r.id !== rule.id);
      order.splice(Math.min(body.position, order.length + 1) - 1, 0, rule);
      renumber(order);
      return { status: 200, body: rules.find((r) => r.id === rule.id) ?? rule };
    },
    "POST /admin/rules/{id}/retire": ({ params }) => {
      const rule = rules.find((r) => r.id === params.id);
      if (rule === undefined) return problem(404, "Not Found");
      if (rule.retired_at === undefined)
        rules = rules.map((r) => (r.id === rule.id ? { ...r, retired_at: iso(Date.now()) } : r));
      return { status: 200, body: rules.find((r) => r.id === rule.id) ?? rule };
    },

    "GET /admin/relays": () => (allowed("manage_settings") ? { status: 200, body: { items: relays } } : forbidden()),
    "POST /admin/relays": ({ body }) => {
      if (!allowed("manage_settings")) return forbidden();
      const refused = (reason: Schema<"RelayProblemReason">) =>
        typed("The relay cannot be joined", "/api/v1/problems/invalid-relay", { reason });
      if (!body.url.startsWith("https://")) return refused("url");
      if (relays.some((r) => r.url === body.url)) return refused("exists");
      const now = Date.now();
      const made: Schema<"AdminRelay"> = {
        ...relay(relays.length + 10, body.url, "pending"),
        id: idAt(now, 47_100 + relays.length),
        protocol: body.protocol ?? (body.url.endsWith("/actor") ? "litepub" : "mastodon"),
        receive: body.receive ?? true,
        state_changed_at: iso(now),
        created_at: iso(now),
      };
      relays = [...relays, made];
      return { status: 201, body: made };
    },
    "POST /admin/relays/{id}/follow": ({ params }) => changeRelay(params.id, ["rejected", "left"], "pending"),
    "DELETE /admin/relays/{id}/follow": ({ params }) => changeRelay(params.id, ["pending", "accepted"], "left"),
    "DELETE /admin/relays/{id}": ({ params }) => {
      const found = relays.find((r) => r.id === params.id);
      if (found === undefined) return problem(404, "Not Found");
      if (found.state === "pending" || found.state === "accepted")
        return typed("The relay cannot be deleted", "/api/v1/problems/invalid-relay", { reason: "state" });
      relays = relays.filter((r) => r.id !== params.id);
      return { status: 204 };
    },

    "GET /admin/login_providers": () =>
      allowed("manage_login_providers") ? { status: 200, body: { items: providers } } : forbidden(),
    "POST /admin/login_providers": ({ body }) => {
      if (!allowed("manage_login_providers")) return forbidden();
      if (body.display_name.trim() === "") return providerRefused("display_name");
      if ((body.kind === "oidc" || body.kind === "mastodon") && (body.issuer ?? "") === "")
        return providerRefused("issuer");
      if (body.kind === "oidc" && !(body.issuer ?? "").startsWith("https://")) return providerRefused("issuer");
      if (body.kind !== "mastodon" && (body.client_id ?? "") === "") return providerRefused("client_id");
      if (body.kind !== "mastodon" && (body.client_secret ?? "") === "") return providerRefused("client_secret");
      providerIds += 1;
      const made = provider(providerIds, body.kind, body.display_name, {
        id: idAt(Date.now(), 48_000 + providerIds),
        ...(body.issuer === undefined ? {} : { issuer: body.issuer }),
        client_id: body.client_id ?? "registered-at-mastodon",
        scopes: [...(body.scopes ?? [])].toSorted(),
        allow_signup: body.allow_signup ?? false,
        allow_link: body.allow_link ?? true,
        claims: [...(body.claims ?? [])],
        organizations: [...(body.organizations ?? [])],
        guilds: [...(body.guilds ?? [])],
      });
      const id = made.id;
      const placed = { ...made, callback_url: `https://${c.host}/auth/external/${id}/callback` };
      providers = [placed, ...providers];
      return { status: 201, body: placed };
    },
    "GET /admin/login_providers/{id}": ({ params }) => {
      const found = providers.find((p) => p.id === params.id);
      return found === undefined ? problem(404, "Not Found") : { status: 200, body: found };
    },
    "PATCH /admin/login_providers/{id}": ({ params, body }) => {
      const found = providers.find((p) => p.id === params.id);
      if (found === undefined) return problem(404, "Not Found");
      if (body.display_name !== undefined && body.display_name.trim() === "") return providerRefused("display_name");
      const next: Schema<"AdminLoginProvider"> = {
        ...found,
        ...(body.display_name === undefined ? {} : { display_name: body.display_name }),
        ...(body.issuer === undefined ? {} : { issuer: body.issuer }),
        ...(body.client_id === undefined ? {} : { client_id: body.client_id }),
        ...(body.scopes === undefined ? {} : { scopes: [...body.scopes].toSorted() }),
        ...(body.allow_signup === undefined ? {} : { allow_signup: body.allow_signup }),
        ...(body.allow_link === undefined ? {} : { allow_link: body.allow_link }),
        ...(body.claims === undefined ? {} : { claims: [...body.claims] }),
        ...(body.organizations === undefined ? {} : { organizations: [...body.organizations] }),
        ...(body.guilds === undefined ? {} : { guilds: [...body.guilds] }),
        revised_at: iso(Date.now()),
      };
      providers = providers.map((p) => (p.id === found.id ? next : p));
      return { status: 200, body: next };
    },
    "POST /admin/login_providers/{id}/enable": ({ params }) => providerState(params.id, "enabled"),
    "POST /admin/login_providers/{id}/disable": ({ params }) => providerState(params.id, "disabled"),
    "POST /admin/login_providers/{id}/rotate_secret": ({ params, body }) => {
      const found = providers.find((p) => p.id === params.id);
      if (found === undefined) return problem(404, "Not Found");
      if (body.client_secret === "") return providerRefused("client_secret");
      return { status: 200, body: found };
    },
    "DELETE /admin/login_providers/{id}": ({ params }) => {
      if (!providers.some((p) => p.id === params.id)) return problem(404, "Not Found");
      providers = providers.filter((p) => p.id !== params.id);
      return { status: 204 };
    },

    "GET /admin/oauth_clients": ({ query }) => {
      if (!allowed("manage_oauth_apps")) return forbidden();
      const registration = query.get("registration");
      const shown = registration === null ? apps : apps.filter((a) => a.registration === registration);
      return { status: 200, body: pageOf(shown, query) };
    },
    "POST /admin/oauth_clients/{id}/disable": ({ params }) => appState(params.id, "disabled"),
    "POST /admin/oauth_clients/{id}/enable": ({ params }) => appState(params.id, "enabled"),
    "GET /admin/oauth_client_host_denials": ({ query }) =>
      allowed("manage_oauth_apps") ? { status: 200, body: pageOf(denials, query) } : forbidden(),
    "POST /admin/oauth_client_host_denials": ({ body }) => {
      if (!allowed("manage_oauth_apps")) return forbidden();
      const host = body.host.trim().toLowerCase();
      if (host === "" || host.includes(":") || host.includes("/"))
        return problem(422, "Unprocessable Content", { detail: "This value is not valid.", field: "host" });
      const found = denials.find((d) => d.host === host);
      if (found !== undefined) return { status: 200, body: found };
      const made: Schema<"AdminOAuthClientHostDenial"> = {
        id: idAt(Date.now(), 49_600 + denials.length),
        host,
        denied_by: c.accountOf(c.viewer),
        denied_by_deleted: false,
        denied_at: iso(Date.now()),
      };
      denials = [made, ...denials];
      return { status: 201, body: made };
    },
    "DELETE /admin/oauth_client_host_denials/{host}": ({ params }) => {
      denials = denials.filter((d) => d.host !== params.host);
      return { status: 204 };
    },
  };

  function changeRelay(
    id: string,
    from: readonly Schema<"RelayState">[],
    to: Schema<"RelayState">,
  ): Answer<"POST /admin/relays/{id}/follow"> {
    const found = relays.find((r) => r.id === id);
    if (found === undefined) return problem(404, "Not Found");
    if (!from.includes(found.state))
      return typed("The relay cannot be changed", "/api/v1/problems/invalid-relay", { reason: "state" });
    const next = { ...found, state: to, state_changed_at: iso(Date.now()) };
    relays = relays.map((r) => (r.id === id ? next : r));
    return { status: 200, body: next };
  }

  function providerState(
    id: string,
    state: Schema<"AdminLoginProviderState">,
  ): Answer<"POST /admin/login_providers/{id}/enable"> {
    const found = providers.find((p) => p.id === id);
    if (found === undefined) return problem(404, "Not Found");
    const next = { ...found, state, state_changed_at: iso(Date.now()) };
    providers = providers.map((p) => (p.id === id ? next : p));
    return { status: 200, body: next };
  }

  function appState(
    id: string,
    state: Schema<"AdminOAuthClientState">,
  ): Answer<"POST /admin/oauth_clients/{id}/enable"> {
    const found = apps.find((a) => a.id === id);
    if (found === undefined) return problem(404, "Not Found");
    if (found.registration === "first_party")
      return problem(422, "Unprocessable Content", { detail: "This value is not valid.", field: "id" });
    const next = { ...found, state };
    apps = apps.map((a) => (a.id === id ? next : a));
    return { status: 200, body: next };
  }

  const startRules = rules.map((r) => r.id);
  const startRelays = relays.map((r) => r.id);
  const startProviders = providers.map((p) => p.id);
  return {
    handlers,
    admin: {
      ruleId: (index) => startRules[index] ?? "none",
      relayId: (index) => startRelays[index] ?? "none",
      providerId: (index) => startProviders[index] ?? "none",
      setting: valueOf,
    },
  };
}
