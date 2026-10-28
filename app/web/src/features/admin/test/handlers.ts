// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the server administration feature (web-client/README.md §7): the job queue, the error groups, the settings, the rules, the
// server's texts, the custom emoji, the webhooks, the login providers, the announcements, the relays, the cost items and their totals, the billing imports, and the supporter program with its plans, subscriptions and payment provider, none by default. The default viewer has no permissions, so the screens' tests start with
// `server.use(operatorHandler(...))` (test/msw/http.ts).
import { http, HttpResponse } from "msw";
import { otherAccountFixture, type Schemas } from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";

/** An instance text as the server answers it, saved at 2026-10-17T00:00:00Z. */
export function instanceTextFixture(key: Schemas["InstanceTextKey"], body: string): Schemas["AdminInstanceText"] {
  return { key, body, revised_at: "2026-10-17T00:00:00Z" };
}

/** The supporter program as the admin API answers it: disabled, on a server that could enable it, with `overrides`. */
export function supporterProgramFixture(
  overrides: Partial<Schemas["AdminSupporterProgram"]> = {},
): Schemas["AdminSupporterProgram"] {
  return {
    enabled: false,
    configured_provider: "stripe",
    sends_email: true,
    commerce_disclosure: true,
    live_subscriptions: 0,
    can_enable: true,
    warnings: [],
    ...overrides,
  };
}

/** The state of the payment provider as the admin API answers it: Stripe in test mode, never reconciled, with `overrides`. */
export function paymentStatusFixture(overrides: Partial<Schemas["PaymentStatus"]> = {}): Schemas["PaymentStatus"] {
  return {
    provider: "stripe",
    mode: "test",
    time_zone: "UTC",
    last_webhook_at: "2026-10-21T00:00:00Z",
    warnings: [],
    ...overrides,
  };
}

/** A supporter plan as the admin API answers it, created at 2026-10-20T00:00:00Z with its prices ready, with `overrides`. */
export function supporterPlanFixture(
  id: string,
  overrides: Partial<Schemas["AdminSupporterPlan"]> = {},
): Schemas["AdminSupporterPlan"] {
  return {
    id,
    name: "Supporter",
    description: "Thanks for the help",
    rank: 1,
    visibility: "public",
    monthly_price: { amount: "500", currency: "JPY" },
    yearly_price: { amount: "5400", currency: "JPY" },
    annual_discount_percent: 10,
    emoji_limit: 5,
    role: { id: `role-${id}`, key: "supporter", position: 10, permissions: [] },
    prices_ready: true,
    retired: false,
    members: 0,
    created_at: "2026-10-20T00:00:00Z",
    revised_at: "2026-10-20T00:00:00Z",
    ...overrides,
  };
}

/** A subscription as the admin API answers it: an active monthly one of @bob, started at 2026-10-20T00:00:00Z, with `overrides`. */
export function supporterSubscriptionFixture(
  id: string,
  overrides: Partial<Schemas["AdminSupporterSubscription"]> = {},
): Schemas["AdminSupporterSubscription"] {
  return {
    id,
    state: "active",
    plan_id: "0199f0e0-0000-7000-8000-0000000000c1",
    plan_name: "Supporter",
    interval: "month",
    price: { amount: "500", currency: "JPY" },
    period_ends_at: "2026-11-20T00:00:00Z",
    account_id: "0192f0c4-7a8e-7000-8000-000000000002",
    username: "bob",
    started_at: "2026-10-20T00:00:00Z",
    ...overrides,
  };
}

/** A local custom emoji as the admin API answers it, registered at 2026-10-20T00:00:00Z, with `overrides`. */
export function adminCustomEmojiFixture(
  id: string,
  shortcode: string,
  overrides: Partial<Schemas["AdminCustomEmoji"]> = {},
): Schemas["AdminCustomEmoji"] {
  return {
    id,
    shortcode,
    local: true,
    uri: `https://my.example/ap/emojis/${id}`,
    url: `https://my.example/media/${id}/r/medium.webp`,
    static_url: `https://my.example/media/${id}/r/medium.webp`,
    enabled: true,
    visible_in_picker: true,
    aliases: [],
    created_by_deleted: false,
    created_at: "2026-10-20T00:00:00Z",
    updated_at: "2026-10-20T00:00:00Z",
    ...overrides,
  };
}

/** A pending request for a new emoji as the admin API answers it, made at 2026-10-21T00:00:00Z by Bob, with `overrides`. */
export function adminCustomEmojiRequestFixture(
  id: string,
  shortcode: string,
  overrides: Partial<Schemas["AdminCustomEmojiRequest"]> = {},
): Schemas["AdminCustomEmojiRequest"] {
  return {
    id,
    kind: "create",
    state: "pending",
    shortcode,
    url: `https://my.example/media/${id}/r/medium.webp`,
    static_url: `https://my.example/media/${id}/r/medium.webp`,
    created_at: "2026-10-21T00:00:00Z",
    requester: otherAccountFixture,
    ...overrides,
  };
}

/** A webhook as the admin API answers it, set up at 2026-10-20T00:00:00Z and enabled, with `overrides`. */
export function webhookFixture(id: string, overrides: Partial<Schemas["AdminWebhook"]> = {}): Schemas["AdminWebhook"] {
  return {
    id,
    url: "https://hooks.example/in",
    description: "Chat",
    events: ["report.created"],
    state: "enabled",
    state_reason: "admin",
    state_changed_at: "2026-10-20T00:00:00Z",
    created_at: "2026-10-20T00:00:00Z",
    updated_at: "2026-10-20T00:00:00Z",
    ...overrides,
  };
}

/** An OpenID Connect login provider as the admin API answers it, set up at 2026-10-20T00:00:00Z and enabled, with `overrides`. */
export function loginProviderFixture(
  id: string,
  overrides: Partial<Schemas["AdminLoginProvider"]> = {},
): Schemas["AdminLoginProvider"] {
  return {
    id,
    kind: "oidc",
    display_name: "Example SSO",
    issuer: "https://sso.example",
    client_id: "my-client",
    scopes: [],
    allow_signup: false,
    allow_link: true,
    claims: [],
    organizations: [],
    guilds: [],
    state: "enabled",
    state_changed_at: "2026-10-20T00:00:00Z",
    callback_url: `https://my.example/auth/external/${id}/callback`,
    linked_users: 0,
    created_at: "2026-10-20T00:00:00Z",
    updated_at: "2026-10-20T00:00:00Z",
    ...overrides,
  };
}

/** A delivery of a webhook as the admin API answers it, made at 2026-10-20T01:00:00Z, with `overrides`. */
export function webhookDeliveryFixture(
  id: string,
  overrides: Partial<Schemas["AdminWebhookDelivery"]> = {},
): Schemas["AdminWebhookDelivery"] {
  return {
    id,
    message_id: `0199f0e0-0000-7000-8000-${id.padStart(12, "0")}`,
    event_type: "report.created",
    status: "pending",
    attempts: 0,
    created_at: "2026-10-20T01:00:00Z",
    ...overrides,
  };
}

/** A relay as the admin API answers it, registered at 2026-10-20T00:00:00Z and joined, with `overrides`. */
export function relayFixture(id: string, overrides: Partial<Schemas["AdminRelay"]> = {}): Schemas["AdminRelay"] {
  return {
    id,
    protocol: "mastodon",
    url: "https://relay.example/inbox",
    inbox_url: "https://relay.example/inbox",
    receive: true,
    state: "accepted",
    state_changed_at: "2026-10-20T00:05:00Z",
    actor_uri: "https://relay.example/actor",
    taken_last_day: 0,
    dropped_last_day: 0,
    domain_suspended: false,
    created_at: "2026-10-20T00:00:00Z",
    ...overrides,
  };
}

/** An OAuth client as the admin API answers it: an app that a user registered at 2026-10-22T00:00:00Z, enabled, with `overrides`. */
export function oauthClientFixture(
  id: string,
  overrides: Partial<Schemas["AdminOAuthClient"]> = {},
): Schemas["AdminOAuthClient"] {
  return {
    id,
    client_id: `client-${id}`,
    name: "Tootle",
    registration: "user",
    confidential: false,
    scope: "read write",
    disabled: false,
    created_at: "2026-10-22T00:00:00Z",
    ...overrides,
  };
}

/** A denied host as the admin API answers it, denied at 2026-10-23T00:00:00Z, with `overrides`. */
export function hostDenialFixture(
  id: string,
  host: string,
  overrides: Partial<Schemas["AdminOAuthClientHostDenial"]> = {},
): Schemas["AdminOAuthClientHostDenial"] {
  return { id, host, denied_at: "2026-10-23T00:00:00Z", ...overrides };
}

/** An announcement as the admin API answers it, written at 2026-10-21T00:00:00Z as a draft, with `overrides`. */
export function adminAnnouncementFixture(
  id: string,
  overrides: Partial<Schemas["AdminAnnouncement"]> = {},
): Schemas["AdminAnnouncement"] {
  return {
    id,
    text: "Maintenance on **Sunday**.",
    content: "<p>Maintenance on <strong>Sunday</strong>.</p>",
    state: "draft",
    created_at: "2026-10-21T00:00:00Z",
    updated_at: "2026-10-21T00:00:00Z",
    ...overrides,
  };
}

/** The ID the default handlers give a new announcement. */
const createdAnnouncementId = "0199f0e0-0000-7000-8000-00000000a665";

/** The secret the default handlers give a new webhook and a rotation. */
export const webhookSecret = "whsec_c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmU=";

/** The instance settings of the form of the server settings screen, at their defaults (internal/domain/instance, setting.go). */
const settingDefaults: readonly Omit<Schemas["AdminSetting"], "value" | "stored">[] = [
  { key: "registration_mode", kind: "enum", default: "approval", choices: ["open", "approval", "invite", "closed"] },
  { key: "max_status_length", kind: "int", default: "500", minimum: 1, maximum: 100000 },
  { key: "default_locale", kind: "locale", default: "en" },
  { key: "local_timeline_access", kind: "enum", default: "everyone", choices: ["off", "logged_in", "everyone"] },
  { key: "federated_timeline_access", kind: "enum", default: "everyone", choices: ["off", "logged_in", "everyone"] },
  { key: "contact_account", kind: "account", default: "" },
  { key: "contact_email", kind: "email", default: "" },
  { key: "thumbnail", kind: "media", default: "" },
  { key: "remote_media_policy", kind: "enum", default: "none", choices: ["none", "avatars", "all"] },
  { key: "remote_media_cache_ttl", kind: "int", default: "86400", minimum: 60, maximum: 31536000 },
  { key: "authorized_fetch", kind: "bool", default: "false" },
  { key: "domain_actions_visibility", kind: "enum", default: "none", choices: ["none", "users", "all"] },
  { key: "domain_actions_rationale_visibility", kind: "enum", default: "none", choices: ["none", "users", "all"] },
  { key: "search_indexing", kind: "enum", default: "deny", choices: ["allow", "deny"] },
  { key: "ai_usage", kind: "enum", default: "deny", choices: ["allow", "deny_training", "deny"] },
  { key: "indexable_default", kind: "bool", default: "false" },
  { key: "security_contact", kind: "uri", default: "" },
  { key: "require_media_description", kind: "bool", default: "false" },
  { key: "link_cards_enabled", kind: "bool", default: "true" },
  { key: "finance_visibility", kind: "enum", default: "private", choices: ["private", "totals", "breakdown"] },
  { key: "finance_viewer_item_names", kind: "bool", default: "false" },
  { key: "finance_viewer_item_memos", kind: "bool", default: "false" },
  { key: "finance_viewer_income", kind: "enum", default: "totals", choices: ["totals", "plans"] },
  { key: "income_visibility", kind: "enum", default: "private", choices: ["private", "totals", "plans"] },
  { key: "supporter_list_visibility", kind: "enum", default: "off", choices: ["off", "public", "signed_in"] },
  { key: "public_counts", kind: "bool", default: "true" },
  { key: "public_operators", kind: "bool", default: "true" },
  { key: "federation_mode", kind: "enum", default: "both", choices: ["off", "inbound", "outbound", "both"] },
  { key: "user_app_registration", kind: "bool", default: "true" },
  { key: "oauth_metadata_document_clients", kind: "bool", default: "true" },
  { key: "dynamic_client_registration", kind: "bool", default: "true" },
  { key: "voice_room_starters", kind: "enum", default: "permitted", choices: ["everyone", "permitted"] },
  { key: "voice_public_summary", kind: "bool", default: "false" },
  { key: "retention_voice_participants", kind: "int", default: "90", minimum: 30, maximum: 3650 },
  { key: "invite_creators", kind: "enum", default: "permitted", choices: ["everyone", "permitted"] },
  { key: "member_invite_max_active", kind: "int", default: "5", minimum: 0, maximum: 100 },
  { key: "member_invite_max_uses", kind: "int", default: "10", minimum: 1, maximum: 1000 },
  { key: "member_invite_max_expires_in", kind: "int", default: "2592000", minimum: 3600, maximum: 31536000 },
];

/** The instance settings as GET /admin/settings answers them, with the `stored` values set and the others at their defaults. */
export function instanceSettingsFixture(
  stored: Readonly<Partial<Record<Schemas["InstanceSettingKey"], string>>> = {},
): Schemas["AdminSettingList"] {
  return {
    items: settingDefaults.map((setting) => {
      const value = stored[setting.key];
      return { ...setting, value: value ?? setting.default, stored: value !== undefined };
    }),
  };
}

/** A rule at `position`, created at 2026-10-16, as the admin API answers it; `overrides` replace its fields. */
export function ruleFixture(
  id: string,
  position: number,
  overrides: Partial<Schemas["AdminRule"]> = {},
): Schemas["AdminRule"] {
  return {
    id,
    position,
    text: `Rule ${String(position)}`,
    created_at: "2026-10-16T00:00:00Z",
    revised_at: "2026-10-16T00:00:00Z",
    ...overrides,
  };
}

/** A cost item as the admin API answers it: a yearly domain from 2026-04-10 at 1,650 yen, recorded then, with `overrides`. */
export function costItemFixture(id: string, overrides: Partial<Schemas["CostItem"]> = {}): Schemas["CostItem"] {
  return {
    id,
    schedule: "years",
    interval: 1,
    starts_on: "2026-04-10",
    name: "example.com",
    provider: "Registrar",
    category: "domain",
    memo: "",
    prices: [
      { effective_on: "2026-04-10", price: { amount: "1650", currency: "JPY" }, revised_at: "2026-04-10T00:00:00Z" },
    ],
    created_at: "2026-04-10T00:00:00Z",
    ...overrides,
  };
}

/** An error group as the admin API answers it: an open panic of the server first seen at 2026-10-20T00:00:00Z, with `overrides`. */
export function errorGroupFixture(id: string, overrides: Partial<Schemas["ErrorGroup"]> = {}): Schemas["ErrorGroup"] {
  return {
    id,
    fingerprint: "0123456789abcdef0123456789abcdef",
    state: "open",
    state_since: "2026-10-20T00:00:00Z",
    source: "server",
    kind: "panic",
    where: "client_api GET /api/v1/statuses/{id}",
    type: "runtime.Error",
    message: "runtime error: invalid memory address or nil pointer dereference",
    count_24h: 3,
    count_7d: 12,
    first_seen_at: "2026-10-20T00:00:00Z",
    last_seen_at: "2026-10-21T05:30:00Z",
    first_version: "0.3.0",
    last_version: "0.3.1",
    ...overrides,
  };
}

/** An error group with its latest occurrence, two hours of counts and no change of state, with `overrides`. */
export function errorGroupDetailFixture(
  id: string,
  overrides: Partial<Schemas["ErrorGroupDetail"]> = {},
): Schemas["ErrorGroupDetail"] {
  return {
    ...errorGroupFixture(id),
    sample: {
      type: "runtime.Error",
      message: "runtime error: invalid memory address or nil pointer dereference",
      stack:
        "goroutine 1 [running]:\ngithub.com/mycroblog/my/internal/app/status.(*Service).Get(...)\n\tgithub.com/mycroblog/my/internal/app/status/get.go:42 +0x32",
      request_id: "01JB0000000000000000000000",
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
    ...overrides,
  };
}

/** The totals that the default handler answers: the periods of the query, each without costs. */
function emptyTotals(url: URL): Schemas["CostTotals"] {
  const granularity = (url.searchParams.get("granularity") ?? "month") as Schemas["CostGranularity"];
  const basis = (url.searchParams.get("basis") ?? "amortized") as Schemas["CostBasis"];
  const to = url.searchParams.get("to") ?? "2026-10-22";
  return {
    granularity,
    basis,
    periods: [
      {
        label: to.slice(0, 7),
        starts_on: `${to.slice(0, 7)}-01`,
        ends_on: to,
        totals: [],
        breakdown: [],
        estimated: false,
      },
    ],
    rules: { week_start: "monday", sources: [] },
  };
}

/** The income totals that the default handler answers: the periods of the query, each without income, and no plan. */
function emptyIncomeTotals(url: URL): Schemas["AdminIncomeTotals"] {
  const granularity = (url.searchParams.get("granularity") ?? "month") as Schemas["CostGranularity"];
  const basis = (url.searchParams.get("basis") ?? "amortized") as Schemas["CostBasis"];
  const to = url.searchParams.get("to") ?? "2026-10-22";
  return {
    granularity,
    basis,
    periods: [
      {
        label: to.slice(0, 7),
        starts_on: `${to.slice(0, 7)}-01`,
        ends_on: to,
        gross: [],
        fee: [],
        net: [],
        balance: [],
      },
    ],
    plans: [],
    rules: { week_start: "monday", providers: [], amortize_after_days: 31 },
  };
}

/** The ID the default POST /admin/cost_items handler gives a new cost item. */
const createdCostItemId = "0199f0e0-0000-7000-8000-0000000000a1";

/** The ID the default POST /admin/rules handler gives a new rule. */
const createdRuleId = "0199f0e0-0000-7000-8000-0000000000f1";

/** The ID the default POST /admin/login_providers handler gives a new provider. */
const createdLoginProviderId = "0199f0e0-0000-7000-8000-0000000000c1";

/** The ID the default POST /admin/webhooks handler gives a new webhook. */
const createdWebhookId = "0199f0e0-0000-7000-8000-0000000000b1";

/** The ID the default POST /admin/relays handler gives a new relay. */
const createdRelayId = "0199f0e0-0000-7000-8000-0000000000d1";

/** The ID the default POST /admin/custom_emojis handler gives a new emoji. */
const createdCustomEmojiId = "0199f0e0-0000-7000-8000-00000000e001";

/** The ID the default POST /admin/roles handler gives a new role. */
export const createdRoleId = "0199f0e0-0000-7000-8000-0000000000c1";

/** The renditions the server asks for of each brand image (BrandRendition). */
const brandRenditionSizes: Record<Schemas["BrandKind"], Schemas["BrandRenditionSize"][]> = {
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

/** A sanitized SVG of `kind` as POST /admin/instance/brand/sanitize answers it: a square icon or a 4:1 logo. */
export function brandSanitizedFixture(
  kind: Schemas["BrandKind"],
  overrides: Partial<Schemas["BrandSanitized"]> = {},
): Schemas["BrandSanitized"] {
  const [width, height] = kind === "icon" ? [64, 64] : [160, 40];
  return {
    svg: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${width} ${height}"><rect width="${width}" height="${height}"/></svg>`,
    view_box_width: width,
    view_box_height: height,
    renditions: brandRenditionSizes[kind],
    dropped: [],
    ...overrides,
  };
}

/** A brand image as GET /admin/instance/brand and PUT /admin/instance/brand/{kind} answer it. */
export function adminBrandImageFixture(
  kind: Schemas["BrandKind"],
  revision: string,
  overrides: Partial<Schemas["AdminBrandImage"]> = {},
): Schemas["AdminBrandImage"] {
  const [width, height] = kind === "icon" ? [64, 64] : [160, 40];
  return {
    revision,
    svg_url: `https://my.example/brand/${revision}/${kind}.svg`,
    width,
    height,
    renditions: brandRenditionSizes[kind].map((size) => ({
      ...size,
      url: `https://my.example/brand/${revision}/${size.rendition}.png`,
    })),
    revised_at: "2026-10-20T00:00:00Z",
    ...(kind === "icon" && { background: "#fffaf2" }),
    ...overrides,
  };
}

export const handlers = [
  // operationIds getAdminBrands, sanitizeAdminBrand, setAdminBrand, clearAdminBrand: the built-in images; an SVG is
  // answered as a sanitized square icon or 4:1 logo of its kind, and a saved image at a new revision.
  http.get(`${apiBase}/admin/instance/brand`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({} satisfies Schemas["AdminBrands"])),
  ),
  http.post(`${apiBase}/admin/instance/brand/sanitize`, async ({ request }) => {
    // Node.js's parser of FormData refuses the files of a body that jsdom's globals took part in: read the text.
    const kind = (/name="kind"\r\n\r\n([a-z_]+)/u.exec(await request.clone().text())?.[1] ??
      "icon") as Schemas["BrandKind"];
    return withViewer(request, () => HttpResponse.json(brandSanitizedFixture(kind)));
  }),
  http.put(`${apiBase}/admin/instance/brand/:kind`, ({ request, params }) => {
    const kind = params["kind"] as Schemas["BrandKind"];
    return withViewer(request, () =>
      HttpResponse.json(adminBrandImageFixture(kind, "0199f0e0-0000-7000-8000-00000000b001")),
    );
  }),
  http.delete(`${apiBase}/admin/instance/brand/:kind`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds createRole, updateRole, deleteRole (the roles themselves are the moderation feature's): a new
  // role is answered as sent; a changed one is the moderator role with the changes sent.
  http.post(`${apiBase}/admin/roles`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["CreateRoleRequest"];
    return withViewer(request, () => HttpResponse.json({ id: createdRoleId, ...body } satisfies Schemas["AdminRole"]));
  }),
  http.patch(`${apiBase}/admin/roles/:role`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["UpdateRoleRequest"];
    const role: Schemas["AdminRole"] = {
      id: "0199f0e0-0000-7000-8000-0000000000c2",
      key: String(params["role"]),
      position: body.position ?? 100,
      permissions: body.permissions ?? ["manage_reports", "manage_accounts", "view_audit"],
      ...(body.badge === undefined ? {} : { badge: body.badge }),
    };
    return withViewer(request, () => HttpResponse.json(role));
  }),
  http.delete(`${apiBase}/admin/roles/:role`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds getAdminQueue, listAdminJobs, listAdminJobAttempts, retryAdminJob: a database queue without jobs.
  http.get(`${apiBase}/admin/queue`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ adapter: "db", pending_listing: true, counts: [] })),
  ),
  http.get(`${apiBase}/admin/jobs`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  http.get(`${apiBase}/admin/jobs/:id/attempts`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] })),
  ),
  http.post(`${apiBase}/admin/jobs/:id/retry`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds listErrorGroups, getErrorGroup, resolveErrorGroup, muteErrorGroup, unmuteErrorGroup: no groups; a
  // group asked for by its ID is an open one, and an action answers the group in its new state.
  http.get(`${apiBase}/admin/error_groups`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] })),
  ),
  http.get(`${apiBase}/admin/error_groups/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(errorGroupDetailFixture(String(params["id"])))),
  ),
  http.post(`${apiBase}/admin/error_groups/:id/resolve`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(errorGroupFixture(String(params["id"]), { state: "resolved", resolved_version: "0.3.1" })),
    ),
  ),
  http.post(`${apiBase}/admin/error_groups/:id/mute`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(errorGroupFixture(String(params["id"]), { state: "muted" }))),
  ),
  http.post(`${apiBase}/admin/error_groups/:id/unmute`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(errorGroupFixture(String(params["id"])))),
  ),
  // operationIds listInstanceTexts, setInstanceText: no texts; a new text is recorded as sent, at 2026-10-17T00:00:00Z.
  http.get(`${apiBase}/admin/instance_texts`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] })),
  ),
  http.put(`${apiBase}/admin/instance_texts/:key`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["SetInstanceTextRequest"];
    return withViewer(request, () =>
      HttpResponse.json(instanceTextFixture(String(params["key"]) as Schemas["InstanceTextKey"], body.body)),
    );
  }),
  // operationIds listInstanceSettings, updateInstanceSettings: every setting at its default; a change is answered
  // with the values sent.
  http.get(`${apiBase}/admin/settings`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(instanceSettingsFixture())),
  ),
  http.patch(`${apiBase}/admin/settings`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["UpdateInstanceSettingsRequest"];
    return withViewer(request, () => HttpResponse.json(instanceSettingsFixture(body.values)));
  }),
  // operationIds listInstanceRules, createInstanceRule, getInstanceRule, updateInstanceRule, moveInstanceRule,
  // retireInstanceRule: no rules; a new or changed rule is answered as sent.
  http.get(`${apiBase}/admin/rules`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminRuleList"])),
  ),
  http.post(`${apiBase}/admin/rules`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["InstanceRuleInput"];
    return withViewer(request, () => HttpResponse.json(ruleFixture(createdRuleId, 1, body)));
  }),
  http.get(`${apiBase}/admin/rules/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(ruleFixture(String(params["id"]), 1))),
  ),
  http.patch(`${apiBase}/admin/rules/:id`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["InstanceRuleInput"];
    return withViewer(request, () =>
      HttpResponse.json(ruleFixture(String(params["id"]), 1, { ...body, revised_at: "2026-10-17T00:00:00Z" })),
    );
  }),
  http.post(`${apiBase}/admin/rules/:id/move`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["MoveInstanceRuleRequest"];
    return withViewer(request, () => HttpResponse.json(ruleFixture(String(params["id"]), body.position)));
  }),
  http.post(`${apiBase}/admin/rules/:id/retire`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(ruleFixture(String(params["id"]), 1, { retired_at: "2026-10-17T00:00:00Z" })),
    ),
  ),
  // operationIds getAdminTheme, setAdminTheme: no theme; a saved theme is answered as sent, at a new revision.
  http.get(`${apiBase}/admin/theme`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ tokens: [], custom_css: "" } satisfies Schemas["AdminTheme"])),
  ),
  http.put(`${apiBase}/admin/theme`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["SetThemeRequest"];
    return withViewer(request, () =>
      HttpResponse.json({
        ...body,
        revision: "0199f0e0-0000-7000-8000-00000000a001",
        revised_at: "2026-10-20T00:00:00Z",
      } satisfies Schemas["AdminTheme"]),
    );
  }),
  // operationIds listAdminCustomEmojis, createCustomEmoji, getAdminCustomEmoji, updateCustomEmoji, deleteCustomEmoji,
  // replaceCustomEmojiImage: no emoji; a new or changed emoji is answered as sent.
  http.get(`${apiBase}/admin/custom_emojis`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminCustomEmojiPage"])),
  ),
  http.post(`${apiBase}/admin/custom_emojis`, async ({ request }) => {
    const { media_id: _, ...body } = (await request.clone().json()) as Schemas["CreateCustomEmojiRequest"];
    return withViewer(request, () =>
      HttpResponse.json(adminCustomEmojiFixture(createdCustomEmojiId, body.shortcode, body)),
    );
  }),
  http.get(`${apiBase}/admin/custom_emojis/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(adminCustomEmojiFixture(String(params["id"]), "blobcat"))),
  ),
  http.patch(`${apiBase}/admin/custom_emojis/:id`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["UpdateCustomEmojiRequest"];
    return withViewer(request, () => HttpResponse.json(adminCustomEmojiFixture(String(params["id"]), "blobcat", body)));
  }),
  http.delete(`${apiBase}/admin/custom_emojis/:id`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.put(`${apiBase}/admin/custom_emojis/:id/image`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(
        adminCustomEmojiFixture(String(params["id"]), "blobcat", { updated_at: "2026-10-21T00:00:00Z" }),
      ),
    ),
  ),
  // operationIds listAdminCustomEmojiRequests, approveCustomEmojiRequest, rejectCustomEmojiRequest: no requests; a
  // decided request is answered approved or rejected with the reason sent.
  http.get(`${apiBase}/admin/custom_emoji_requests`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminCustomEmojiRequestPage"])),
  ),
  http.post(`${apiBase}/admin/custom_emoji_requests/:id/approve`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(
        adminCustomEmojiRequestFixture(String(params["id"]), "blobcat", {
          state: "approved",
          decided_at: "2026-10-22T00:00:00Z",
        }),
      ),
    ),
  ),
  http.post(`${apiBase}/admin/custom_emoji_requests/:id/reject`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["RejectCustomEmojiRequestRequest"];
    return withViewer(request, () =>
      HttpResponse.json(
        adminCustomEmojiRequestFixture(String(params["id"]), "blobcat", {
          state: "rejected",
          decided_at: "2026-10-22T00:00:00Z",
          ...(body.reason === undefined ? {} : { reason: body.reason }),
        }),
      ),
    );
  }),
  // operationIds listAdminWebhooks, createWebhook, getAdminWebhook, updateWebhook, deleteWebhook, enableWebhook,
  // disableWebhook, rotateWebhookSecret, testWebhook, listWebhookDeliveries, redeliverWebhookDelivery: no webhooks
  // and no deliveries; a new or changed webhook is answered as sent.
  http.get(`${apiBase}/admin/webhooks`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminWebhookList"])),
  ),
  http.post(`${apiBase}/admin/webhooks`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["CreateWebhookRequest"];
    return withViewer(request, () =>
      HttpResponse.json({
        webhook: webhookFixture(createdWebhookId, body),
        secret: webhookSecret,
      } satisfies Schemas["AdminWebhookCreated"]),
    );
  }),
  http.get(`${apiBase}/admin/webhooks/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(webhookFixture(String(params["id"])))),
  ),
  http.patch(`${apiBase}/admin/webhooks/:id`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["UpdateWebhookRequest"];
    return withViewer(request, () => HttpResponse.json(webhookFixture(String(params["id"]), body)));
  }),
  http.delete(`${apiBase}/admin/webhooks/:id`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.post(`${apiBase}/admin/webhooks/:id/enable`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(webhookFixture(String(params["id"])))),
  ),
  http.post(`${apiBase}/admin/webhooks/:id/disable`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(webhookFixture(String(params["id"]), { state: "disabled" }))),
  ),
  http.post(`${apiBase}/admin/webhooks/:id/rotate_secret`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ secret: webhookSecret } satisfies Schemas["AdminWebhookSecret"])),
  ),
  http.post(`${apiBase}/admin/webhooks/:id/test`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(webhookDeliveryFixture("1", { event_type: "webhook.test" }))),
  ),
  http.get(`${apiBase}/admin/webhooks/:id/deliveries`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminWebhookDeliveryPage"])),
  ),
  http.post(`${apiBase}/admin/webhooks/:id/deliveries/:delivery_id/redeliver`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(webhookDeliveryFixture(String(params["delivery_id"])))),
  ),
  // operationIds listAdminLoginProviders, createLoginProvider, getAdminLoginProvider, updateLoginProvider,
  // deleteLoginProvider, enableLoginProvider, disableLoginProvider, rotateLoginProviderSecret: no providers; a new or
  // changed provider is answered as sent.
  http.get(`${apiBase}/admin/login_providers`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminLoginProviderList"])),
  ),
  http.post(`${apiBase}/admin/login_providers`, async ({ request }) => {
    const { client_secret: _, ...body } = (await request.clone().json()) as Schemas["CreateLoginProviderRequest"];
    return withViewer(request, () => HttpResponse.json(loginProviderFixture(createdLoginProviderId, body)));
  }),
  http.get(`${apiBase}/admin/login_providers/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(loginProviderFixture(String(params["id"])))),
  ),
  http.patch(`${apiBase}/admin/login_providers/:id`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["UpdateLoginProviderRequest"];
    return withViewer(request, () => HttpResponse.json(loginProviderFixture(String(params["id"]), body)));
  }),
  http.delete(`${apiBase}/admin/login_providers/:id`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.post(`${apiBase}/admin/login_providers/:id/enable`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(loginProviderFixture(String(params["id"])))),
  ),
  http.post(`${apiBase}/admin/login_providers/:id/disable`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(loginProviderFixture(String(params["id"]), { state: "disabled" }))),
  ),
  http.post(`${apiBase}/admin/login_providers/:id/rotate_secret`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(loginProviderFixture(String(params["id"])))),
  ),
  // operationIds listAdminRelays, createRelay, followRelay, unfollowRelay, deleteRelay: no relays; a new relay is
  // pending with what was sent, a relay joined again is pending and one left is left.
  http.get(`${apiBase}/admin/relays`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminRelayList"])),
  ),
  http.post(`${apiBase}/admin/relays`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["CreateRelayRequest"];
    const { actor_uri: _, ...pending } = relayFixture(createdRelayId, {
      url: body.url,
      inbox_url: body.url,
      protocol: body.protocol ?? "mastodon",
      receive: body.receive ?? true,
      state: "pending",
    });
    return withViewer(request, () => HttpResponse.json(pending));
  }),
  http.post(`${apiBase}/admin/relays/:id/follow`, ({ request, params }) => {
    const { actor_uri: _, ...pending } = relayFixture(String(params["id"]), { state: "pending" });
    return withViewer(request, () => HttpResponse.json(pending));
  }),
  http.post(`${apiBase}/admin/relays/:id/unfollow`, ({ request, params }) => {
    const { actor_uri: _, ...left } = relayFixture(String(params["id"]), { state: "left" });
    return withViewer(request, () => HttpResponse.json(left));
  }),
  http.delete(`${apiBase}/admin/relays/:id`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds listAdminOAuthClients, disableOAuthClient, enableOAuthClient, listOAuthClientHostDenials,
  // denyOAuthClientHost, allowOAuthClientHost: no clients and no denied hosts; the changes are answered as asked.
  http.get(`${apiBase}/admin/oauth_clients`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminOAuthClientPage"])),
  ),
  http.post(`${apiBase}/admin/oauth_clients/:id/disable`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(oauthClientFixture(String(params["id"]), { disabled: true }))),
  ),
  http.post(`${apiBase}/admin/oauth_clients/:id/enable`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(oauthClientFixture(String(params["id"])))),
  ),
  http.get(`${apiBase}/admin/oauth_client_host_denials`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminOAuthClientHostDenialPage"])),
  ),
  http.post(`${apiBase}/admin/oauth_client_host_denials`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.delete(`${apiBase}/admin/oauth_client_host_denials/:host`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds listAdminAnnouncements, createAdminAnnouncement, getAdminAnnouncement, updateAdminAnnouncement,
  // publishAdminAnnouncement, withdrawAdminAnnouncement: no announcements; a new or changed one is answered as sent.
  http.get(`${apiBase}/admin/announcements`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminAnnouncementList"])),
  ),
  http.post(`${apiBase}/admin/announcements`, async ({ request }) => {
    const { publish, ...body } = (await request.clone().json()) as Schemas["AdminAnnouncementCreateRequest"];
    return withViewer(request, () =>
      HttpResponse.json(
        adminAnnouncementFixture(createdAnnouncementId, { ...body, state: publish ? "published" : "draft" }),
      ),
    );
  }),
  http.get(`${apiBase}/admin/announcements/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(adminAnnouncementFixture(String(params["id"])))),
  ),
  http.put(`${apiBase}/admin/announcements/:id`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["AdminAnnouncementInput"];
    return withViewer(request, () => HttpResponse.json(adminAnnouncementFixture(String(params["id"]), body)));
  }),
  http.post(`${apiBase}/admin/announcements/:id/publish`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(
        adminAnnouncementFixture(String(params["id"]), { state: "published", published_at: "2026-10-21T00:00:00Z" }),
      ),
    ),
  ),
  http.post(`${apiBase}/admin/announcements/:id/withdraw`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(adminAnnouncementFixture(String(params["id"]), { state: "withdrawn" })),
    ),
  ),
  // operationIds listCostItems, createCostItem, getCostItem, updateCostItem, reviseCostItemPrice, setCostItemEnd,
  // cancelCostItem, getCostTotals, listBillingImports, runBillingImport: no items, no costs in the periods asked for,
  // and no billing sources; a new or changed item is answered as sent.
  http.get(`${apiBase}/admin/cost_items`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["CostItemPage"])),
  ),
  http.post(`${apiBase}/admin/cost_items`, async ({ request }) => {
    const { price, ends_on, memo, ...body } = (await request.clone().json()) as Schemas["CostItemCreateRequest"];
    return withViewer(request, () =>
      HttpResponse.json(
        costItemFixture(createdCostItemId, {
          ...body,
          memo: memo ?? "",
          ...(ends_on === undefined ? {} : { ends_on }),
          prices: [{ effective_on: body.starts_on, price, revised_at: "2026-10-22T00:00:00Z" }],
        }),
      ),
    );
  }),
  http.get(`${apiBase}/admin/cost_items/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(costItemFixture(String(params["id"])))),
  ),
  http.patch(`${apiBase}/admin/cost_items/:id`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["CostItemLabelRequest"];
    return withViewer(request, () => HttpResponse.json(costItemFixture(String(params["id"]), body)));
  }),
  http.post(`${apiBase}/admin/cost_items/:id/price_revisions`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["CostPriceRevisionRequest"];
    const item = costItemFixture(String(params["id"]));
    return withViewer(request, () =>
      HttpResponse.json({ ...item, prices: [...item.prices, { ...body, revised_at: "2026-10-22T00:00:00Z" }] }),
    );
  }),
  http.put(`${apiBase}/admin/cost_items/:id/end`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["CostItemEndRequest"];
    return withViewer(request, () => HttpResponse.json(costItemFixture(String(params["id"]), body)));
  }),
  http.post(`${apiBase}/admin/cost_items/:id/cancel`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.get(`${apiBase}/admin/cost_totals`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(emptyTotals(new URL(request.url)))),
  ),
  http.get(`${apiBase}/admin/billing_imports`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["BillingImportList"])),
  ),
  http.get(`${apiBase}/admin/income_totals`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(emptyIncomeTotals(new URL(request.url)))),
  ),
  http.get(`${apiBase}/admin/income_movements`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["IncomeMovementPage"])),
  ),
  http.post(`${apiBase}/admin/billing_imports/:source/runs`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds getSupporterProgram, setSupporterProgram, listAdminSupporterPlans, createSupporterPlan,
  // updateSupporterPlan, retireSupporterPlan, listSupporterSubscriptions, cancelSupporterSubscription, getPaymentStatus,
  // runPaymentReconcile: a disabled program without plans or subscriptions, and Stripe in test mode; a change is
  // answered as sent.
  http.get(`${apiBase}/admin/supporter_program`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(supporterProgramFixture())),
  ),
  http.put(`${apiBase}/admin/supporter_program`, async ({ request }) => {
    const { enabled } = (await request.clone().json()) as Schemas["AdminSupporterProgramRequest"];
    return withViewer(request, () =>
      HttpResponse.json(supporterProgramFixture({ enabled, ...(enabled ? { provider: "stripe" } : {}) })),
    );
  }),
  http.get(`${apiBase}/admin/supporter_plans`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminSupporterPlanList"])),
  ),
  http.post(`${apiBase}/admin/supporter_plans`, async ({ request }) => {
    const { role, description, ...body } = (await request.clone().json()) as Schemas["AdminSupporterPlanCreateRequest"];
    return withViewer(request, () =>
      HttpResponse.json(
        supporterPlanFixture("0199f0e0-0000-7000-8000-0000000000c9", {
          ...body,
          description: description ?? "",
          prices_ready: false,
          role: { id: "role-new", key: role, position: 10, permissions: [] },
        }),
      ),
    );
  }),
  http.patch(`${apiBase}/admin/supporter_plans/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(supporterPlanFixture(String(params["id"])))),
  ),
  http.post(`${apiBase}/admin/supporter_plans/:id/retire`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.get(`${apiBase}/admin/supporter_subscriptions`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["AdminSupporterSubscriptionPage"])),
  ),
  http.post(`${apiBase}/admin/supporter_subscriptions/:id/cancel`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.get(`${apiBase}/admin/payment_status`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(paymentStatusFixture())),
  ),
  http.post(`${apiBase}/admin/payment_reconciles`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
];
