// SPDX-License-Identifier: AGPL-3.0-only

// The reader's personal settings in the fake API (ADR-0171 §5), the operations of the settings screens other than the
// profile's and the lists of blocks and mutes, which the fake's people give it (api.ts): keyword filters, role badges,
// featured hashtags, the cells of the notification settings and the notification policy, the policy that deletes old
// posts and its estimate, imports of a CSV file, the account's aliases and its move, its deactivation and deletion, and
// the moderators' actions on the account with their appeals. Each is kept in memory and answered as the server answers;
// `FakeSettings` is what a fake starts with, which a test replaces in part. The filters also judge the statuses the
// rest of the fake answers a read with (`judged`): they fold or leave out what their words match.
//
// An import started stays in progress for a moment (`importSeconds`), then reads as finished. A confirmation (a move,
// a deactivation, a deletion) holds with the reader's password, a passkey or the confirmation of an external login.

import { day, idAt, notFound, page, problem, type PageQuery } from "./common";
import type { Answer, Handlers, Schema } from "./spec";

type KeywordFilter = Schema<"KeywordFilter">;
type FilterContext = Schema<"KeywordFilterContext">;
type Status = Schema<"Status">;
type BadgeChoice = Schema<"BadgeChoice">;
type FeaturedHashtag = Schema<"FeaturedHashtag">;
type AutoDeletePolicy = Schema<"AutoDeletePolicy">;
type AutoDeleteLimit = Schema<"AutoDeleteLimit">;
type DataImport = Schema<"DataImport">;
type AccountAlias = Schema<"AccountAlias">;
type ModerationAction = Schema<"ModerationAction">;
type OwnHookAppeal = Schema<"OwnHookAppeal">;
type NotificationPolicy = Schema<"NotificationPolicy">;
type DeactivationPreview = Schema<"DeactivationPreview">;

/** A filter as the seed writes it: the server's filter, without its ID. */
type FakeFilter = Omit<KeywordFilter, "id" | "expired" | "created_at"> & { createdDaysAgo?: number };

/**
 * A row of the notification settings as the seed writes it: whether the reader gets its notifications in the app and
 * by email when the fake starts, `fixed` for a channel the server keeps on for everyone; without `email`, the row is
 * never emailed. Its types are `types`, or the one of the group's own name.
 */
export type FakeNotificationRow = {
  group: Schema<"NotificationGroup">;
  section: Schema<"NotificationSettingRow">["section"];
  types?: readonly Schema<"NotificationType">[];
  in_app: boolean | "fixed";
  email?: boolean | "fixed";
};

const ordinaryRow = (
  group: Schema<"NotificationGroup">,
  more: Partial<FakeNotificationRow> = {},
): FakeNotificationRow => ({
  group,
  section: "ordinary",
  in_app: true,
  email: false,
  ...more,
});

/**
 * The rows a member of a server gets, with the server's defaults: every kind from other people, on in the app and off
 * by email (new posts and approval requests are never emailed), then the server's own notices, of which the
 * moderators' and the closure's are always in the app, and the closure's and the supporter plan's always emailed.
 */
export const memberNotificationRows: readonly FakeNotificationRow[] = [
  ordinaryRow("mention"),
  ordinaryRow("follow"),
  ordinaryRow("follow_request"),
  ordinaryRow("follow_accepted"),
  ordinaryRow("boost"),
  ordinaryRow("favourite"),
  ordinaryRow("reaction"),
  ordinaryRow("quote"),
  {
    group: "interaction_request",
    section: "ordinary",
    types: ["reply_request", "boost_request", "quote_request", "interaction_accepted", "interaction_rejected"],
    in_app: true,
  },
  ordinaryRow("poll_closed"),
  ordinaryRow("account_moved"),
  ordinaryRow("voice", { types: ["voice_room_started", "voice_room_invited"] }),
  ordinaryRow("event", {
    types: [
      "event_invited",
      "event_schedule_confirmed",
      "event_changed",
      "event_cancelled",
      "event_reminder",
      "event_started",
      "event_room_unavailable",
    ],
  }),
  { group: "status", section: "ordinary", in_app: true },
  {
    group: "moderation",
    section: "instance",
    types: [
      "moderation_notice",
      "report_resolved",
      "appeal_decided",
      "relationships_severed",
      "custom_emoji_request_decided",
      "hook_appeal_decided",
    ],
    in_app: "fixed",
    email: false,
  },
  { group: "service_closure", section: "instance", in_app: "fixed", email: "fixed" },
  {
    group: "supporter",
    section: "instance",
    types: [
      "supporter_payment_failed",
      "supporter_subscription_ended",
      "supporter_plan_retired",
      "supporter_service_closure",
    ],
    in_app: "fixed",
    email: "fixed",
  },
  { group: "announcement", section: "instance", in_app: true },
  { group: "auto_delete_limit", section: "instance", in_app: true, email: true },
  { group: "scheduled_status_failed", section: "instance", in_app: true },
];

/**
 * The rows a moderator and administrator gets besides a member's, after the kinds from other people: one a kind of
 * what awaits them, on in the app and by email.
 */
export const moderatorNotificationRows: readonly FakeNotificationRow[] = (
  [
    "admin_report",
    "admin_sign_up",
    "admin_appeal",
    "admin_webhook_disabled",
    "admin_hook_failing",
    "admin_hook_appeal",
    "admin_domain_list_failing",
    "admin_custom_emoji_request",
    "admin_account_moved",
    "admin_service_closure_due",
    "admin_error_group",
  ] as const
).map((group) => ({ group, section: "moderator", in_app: true, email: true }));

/** What the fake's personal settings hold when it starts. */
export type FakeSettings = {
  /** The reader's keyword filters, oldest first; an expiry in the past makes one expired. */
  filters: readonly FakeFilter[];
  /**
   * How a filter's save is answered, where not as the server would: `title` refuses the name of every filter made or
   * changed, naming the field; `waits` never answers.
   */
  filterSave?: "title" | "waits" | undefined;
  /** The badges of the roles the reader holds. */
  badges: readonly Omit<BadgeChoice, "role_id">[];
  /** The hashtags the reader features, by name. */
  featured: readonly string[];
  /** The notification policy. */
  policy: NotificationPolicy;
  /** The rows of the notification settings the reader gets, in the order shown, each cell as it starts. */
  notificationRows: readonly FakeNotificationRow[];
  /** The policy that deletes old posts as saved, and the server's maximum over it (none: the server sets no maximum). */
  autoDelete: AutoDeletePolicy;
  autoDeleteLimit: AutoDeleteLimit | undefined;
  /** The imports the reader started, newest first. */
  imports: readonly Omit<DataImport, "id">[];
  /** The accounts the reader says are also theirs, by handle. */
  aliases: readonly string[];
  /** Why the account cannot be deactivated; none if it can. */
  deactivationRefusal: DeactivationPreview["refusal"];
  /** The actions of the moderators on the reader's account, newest first, with the appeals made. */
  moderation: readonly Omit<ModerationAction, "id">[];
};

/**
 * What the fake's settings hold unless a test gives others: three filters of which one expired; a moderator's badge
 * shown and a supporter's not; two featured hashtags; a policy that holds the new and the unsolicited back; seven
 * rows of notification settings, of kinds from other people, on in the app and off by email; a policy
 * that deletes posts older than half a year under a server maximum of a year, which does not let posts with media be
 * kept and so binds the saved policy; a finished import; an alias; a limit of
 * the account in effect that can be appealed and an earlier action whose appeal was decided.
 */
export const defaultSettings: FakeSettings = {
  filters: [
    {
      title: "Spoilers",
      action: "warn",
      contexts: ["home", "public", "thread"],
      keywords: [
        { keyword: "finale", whole_word: true },
        { keyword: "season 3", whole_word: false },
      ],
      createdDaysAgo: 20,
    },
    {
      title: "Election season",
      action: "hide",
      contexts: ["home", "notifications", "public"],
      keywords: [
        { keyword: "election", whole_word: true },
        { keyword: "ballot", whole_word: true },
        { keyword: "polling", whole_word: false },
      ],
      expires_at: "+30d",
      createdDaysAgo: 6,
    },
    {
      title: "Marathon week",
      action: "warn",
      contexts: ["home"],
      keywords: [{ keyword: "marathon", whole_word: true }],
      expires_at: "-5d",
      createdDaysAgo: 40,
    },
  ],
  badges: [
    { label: "Moderator", color: "blue", displayed: true },
    { label: "Supporter", color: "pink", displayed: false },
  ],
  featured: ["baking", "photography"],
  policy: {
    not_following: "accept",
    not_follower: "accept",
    new_account: "filter",
    unsolicited_private_mention: "filter",
    limited_account: "filter",
  },
  notificationRows: (["mention", "follow", "follow_request", "favourite", "reaction", "boost", "quote"] as const).map(
    (group) => ordinaryRow(group),
  ),
  autoDelete: {
    enabled: true,
    min_age_days: 180,
    keep_pinned: true,
    keep_bookmarked: true,
    keep_favourited: false,
    // The server's maximum does not let posts with media be kept, so the saved policy is bound by it.
    keep_media: true,
    keep_polls: false,
    keep_articles: true,
    keep_visibilities: ["private"],
    keep_min_boosts: 0,
    keep_min_favourites: 0,
  },
  autoDeleteLimit: {
    max_age_days: 365,
    allow_pinned: true,
    kept_pins: 5,
    allow_bookmarked: true,
    allow_favourited: true,
    allow_media: false,
    allow_polls: true,
    allow_articles: true,
    allow_visibilities: true,
    allow_thresholds: false,
  },
  imports: [
    {
      kind: "following",
      mode: "merge",
      state: "finished",
      created_at: "-2d",
      total: 120,
      processed: 120,
      failed: 3,
    },
  ],
  aliases: ["yuki@old.example"],
  deactivationRefusal: undefined,
  moderation: [
    {
      kind: "limit",
      kind_label: "Limited",
      reason_category: "violation",
      reason_category_label: "Violation of the rules",
      public_reason: "Repeated replies that took other people's threads off their subject.",
      status_ids: [],
      state: "active",
      issued_at: "-3d",
      expires_at: "+11d",
      appealable: true,
      appeal_deadline: "+4d",
    },
    {
      kind: "mark_statuses_sensitive",
      kind_label: "Posts marked sensitive",
      reason_category: "other",
      reason_category_label: "Other",
      public_reason: "Photos of a medical procedure without a content warning.",
      status_ids: [],
      state: "lifted",
      issued_at: "-60d",
      lifted_at: "-52d",
      appealable: false,
      appeal: {
        id: "appeal",
        text: "The photos are of my own recovery and I added a warning to each.",
        state: "overturned",
        submitted_at: "-58d",
        decided_at: "-52d",
        decision_reason: "On a second look the posts carried the warning. The action is lifted.",
      },
    },
  ],
};

/** What the settings handlers need of the rest of the fake. */
export type SettingsEnv = {
  /** When the fake started, in ms. */
  started: number;
  /** The origin of the server, for the URLs of hashtags. */
  origin: string;
  /** The reader's password, which confirms a move, a deactivation and a deletion. */
  password: string;
  /** The ID of the reader's account; none for a visitor, who has no filters. */
  viewerId: string | undefined;
  /** The ID of the account of a person of the seed, if there is one. */
  accountExists: (id: string) => boolean;
  /** Some of the reader's own posts, newest first, as statuses (what an estimate shows as examples). */
  ownStatuses: () => Schema<"Status">[];
  /** The settings it starts with. */
  settings: FakeSettings;
};

/** A time of the seed: an offset from the start (`+30d`, `-5d`) or a date. */
const timeOf = (started: number, text: string): string => {
  const offset = /^([+-])(\d+)d$/.exec(text);
  return offset === null
    ? text
    : new Date(started + (offset[1] === "-" ? -1 : 1) * Number(offset[2]) * day).toISOString();
};

/** How long a started import stays in progress. */
const importSeconds = 3;

/** The estimate of a policy that keeps nothing in particular: the older the limit, the fewer the posts. */
const estimate = (days: number) => Math.max(0, Math.round(1500 / (1 + days / 20)));

/** The pieces of the settings the rest of the fake reads back. */
export type SettingsState = {
  handlers: Handlers;
  /**
   * The statuses of a read as the reader's keyword filters leave them where `context` shows them (ADR-0157 §6): one
   * that a `hide` filter matches is left out, and one that only `warn` filters match carries them in its `filtered`,
   * a boost those of the post it boosts; a quoted post carries its own and never leaves out the post that quotes it.
   * `opened` is the post the reader opened, which a `hide` filter folds too. The reader's own posts and boosts, and
   * what they quote, never match; nor does anything for a visitor.
   */
  judged: (statuses: readonly Status[], context: FilterContext, opened?: boolean) => Status[];
  /** The reader's featured hashtags, as the profile lists them. */
  featured: () => FeaturedHashtag[];
  /** The badges the reader shows on their profile now. */
  shownBadges: () => { role_id: string; label: string; color: Schema<"BadgeColor"> }[];
  /** The ID of the reader's filter at `index`, oldest first; none past the last. */
  filterId: (index: number) => string | undefined;
};

/** Creates the state of the personal settings from `env.settings`, and the handlers that answer for it. */
export function createSettings(env: SettingsEnv): SettingsState {
  const { started, settings } = env;
  const at = (text: string) => timeOf(started, text);
  const refused = (title: string, type: string, status = 403) =>
    problem(status, title, { type: `/api/v1/problems/${type}` });

  // ---- Keyword filters ----------------------------------------------------------------------------------------------

  let filterCount = 0;
  const makeFilter = (f: Omit<KeywordFilter, "id" | "expired" | "created_at">, createdAt: string): KeywordFilter => {
    filterCount += 1;
    const expires = f.expires_at === undefined ? undefined : at(f.expires_at);
    const { expires_at: _drop, ...rest } = f;
    return {
      ...rest,
      id: idAt(started, 100_000 + filterCount),
      ...(expires === undefined ? {} : { expires_at: expires }),
      expired: expires !== undefined && Date.parse(expires) <= Date.now(),
      created_at: createdAt,
    };
  };
  const filters: KeywordFilter[] = settings.filters.map(({ createdDaysAgo = 1, ...f }) =>
    makeFilter(f, new Date(started - createdDaysAgo * day).toISOString()),
  );
  const filterLimits = { filters: 100, keywords: 1000, perFilter: 100 };
  const checkFilter = (f: Schema<"KeywordFilterRequest">, others: readonly KeywordFilter[]) => {
    const title = f.title.trim();
    const invalid = (field: string, detail: string) => problem(422, "This could not be done.", { detail, field });
    if (Array.from(title).length < 1 || Array.from(title).length > 50 || /[\p{Cc}]/u.test(title))
      return invalid("title", "The name is 1 to 50 characters on one line.");
    if (settings.filterSave === "title") return invalid("title", "This name cannot be used.");
    if (f.contexts.length === 0) return invalid("contexts", "A filter applies somewhere.");
    if (f.keywords.length < 1) return invalid("keywords", "A filter has at least one word.");
    const long = f.keywords.findIndex((k) => Array.from(k.keyword.trim()).length > 100);
    if (long >= 0) return invalid(`keywords[${String(long)}].keyword`, "A word is up to 100 characters.");
    const words = others.reduce((n, o) => n + o.keywords.length, 0) + f.keywords.length;
    if (f.keywords.length > filterLimits.perFilter || words > filterLimits.keywords)
      return refused("You have as many words in your filters as you may", "too-many-filter-keywords", 422);
    return undefined;
  };
  const trimmed = (f: Schema<"KeywordFilterRequest">): Schema<"KeywordFilterRequest"> => ({
    ...f,
    title: f.title.trim(),
    keywords: f.keywords.map((k) => ({ ...k, keyword: k.keyword.trim() })),
  });
  const nowFilter = (f: KeywordFilter): KeywordFilter =>
    f.expires_at === undefined ? f : { ...f, expired: Date.parse(f.expires_at) <= Date.now() };

  // What a filter's words are looked for in, each text on its own: the text without its markup, the content warning,
  // the poll's options and the descriptions of the media. A word matches wherever a text holds it, whatever its case:
  // whole words, and the forms of a character the server takes as one (ADR-0157 §4, §5), are the server's alone.
  const textsOf = (s: Status): string[] =>
    [
      s.content.replace(/<\/p>|<br\s*\/?>/g, " ").replace(/<[^>]*>/g, ""),
      s.spoiler_text,
      ...(s.poll?.options.map((option) => option.title) ?? []),
      ...s.media_attachments.flatMap((m) => m.description ?? []),
    ].map((text) => text.toLowerCase());
  /** The filters in effect in `context` that `s` matches, each with the words of it that matched. */
  const matchesOf = (s: Status, context: FilterContext) => {
    const texts = textsOf(s);
    return filters
      .map(nowFilter)
      .filter((f) => !f.expired && f.contexts.includes(context))
      .flatMap((f) => {
        const words = f.keywords
          .map((k) => k.keyword)
          .filter((word) => word !== "" && texts.some((text) => text.includes(word.toLowerCase())));
        return words.length === 0 ? [] : [{ filter: f, words }];
      });
  };
  type Match = ReturnType<typeof matchesOf>[number];
  const marked = (s: Status, matches: readonly Match[]): Status =>
    matches.length === 0
      ? s
      : {
          ...s,
          filtered: matches.map((m) => ({
            filter: { id: m.filter.id, title: m.filter.title },
            keyword_matches: m.words,
          })),
        };
  const judged = (statuses: readonly Status[], context: FilterContext, opened = false): Status[] =>
    statuses.flatMap((row) => {
      const shown = row.boost ?? row;
      const own = (s: Status) => s.account.id === env.viewerId;
      if (env.viewerId === undefined || own(row) || own(shown)) return [row];
      const matches = matchesOf(shown, context);
      if (!opened && matches.some((m) => m.filter.action === "hide")) return [];
      const quoted = shown.quote?.quoted_status;
      const inQuote = quoted === undefined || own(quoted) ? [] : matchesOf(quoted, context);
      const inner =
        shown.quote === undefined || quoted === undefined || inQuote.length === 0
          ? shown
          : { ...shown, quote: { ...shown.quote, quoted_status: marked(quoted, inQuote) } };
      return [marked(row.boost === undefined ? inner : { ...row, boost: inner }, matches)];
    });

  // ---- Badges and featured hashtags --------------------------------------------------------------------------------

  const badges: BadgeChoice[] = settings.badges.map((b, i) => ({ ...b, role_id: idAt(started, 110_000 + i) }));
  const featuredNames = [...settings.featured];
  const featuredBody = (): FeaturedHashtag[] =>
    featuredNames.map((name, i) => ({
      name,
      url: `${env.origin}/tags/${name}`,
      statuses_count: 7 - i * 2,
      last_status_at: new Date(started - (i + 1) * day).toISOString(),
    }));

  // ---- Notification settings and policy ----------------------------------------------------------------------------

  // A cell the reader may change is kept by its row and channel; a fixed one is on, and a row without email has none.
  const seeded = settings.notificationRows;
  const cells = new Map<string, boolean>(
    seeded.flatMap((row) => [
      ...(row.in_app === "fixed" ? [] : [[`${row.group}/in_app`, row.in_app] as const]),
      ...(row.email === undefined || row.email === "fixed" ? [] : [[`${row.group}/email`, row.email] as const]),
    ]),
  );
  const cellOf = (group: string, channel: "in_app" | "email", seed: boolean | "fixed") =>
    seed === "fixed"
      ? { enabled: true, fixed: true }
      : { enabled: cells.get(`${group}/${channel}`) ?? seed, fixed: false };
  const rows = (): Schema<"NotificationSettingRow">[] =>
    seeded.map((row) => ({
      group: row.group,
      section: row.section,
      types: [...(row.types ?? [row.group as Schema<"NotificationType">])],
      in_app: cellOf(row.group, "in_app", row.in_app),
      ...(row.email === undefined ? {} : { email: cellOf(row.group, "email", row.email) }),
    }));
  let policy: NotificationPolicy = settings.policy;

  // ---- The policy that deletes old posts ---------------------------------------------------------------------------

  let autoDelete: AutoDeletePolicy = settings.autoDelete;
  const limit = settings.autoDeleteLimit;
  /** What the server's maximum makes of a policy: on, no older than the maximum, keeping only what it allows. */
  const settingsOf = (saved: AutoDeletePolicy): Schema<"AutoDeleteSettings"> => {
    if (limit === undefined) return saved;
    const effective: AutoDeletePolicy = {
      ...saved,
      enabled: true,
      min_age_days: Math.min(saved.min_age_days, limit.max_age_days),
      keep_pinned: saved.keep_pinned && limit.allow_pinned,
      keep_bookmarked: saved.keep_bookmarked && limit.allow_bookmarked,
      keep_favourited: saved.keep_favourited && limit.allow_favourited,
      keep_media: saved.keep_media && limit.allow_media,
      keep_polls: saved.keep_polls && limit.allow_polls,
      keep_articles: (saved.keep_articles ?? true) && (limit.allow_articles ?? true),
      keep_visibilities: limit.allow_visibilities ? saved.keep_visibilities : [],
      keep_min_boosts: limit.allow_thresholds ? saved.keep_min_boosts : 0,
      keep_min_favourites: limit.allow_thresholds ? saved.keep_min_favourites : 0,
    };
    const bound =
      JSON.stringify(effective) !== JSON.stringify({ ...saved, keep_articles: saved.keep_articles ?? true });
    return {
      ...saved,
      keep_articles: saved.keep_articles ?? true,
      server_limit: {
        ...limit,
        effective,
        bound,
        ...(bound ? { enforce_from: new Date(started + 14 * day).toISOString() } : {}),
      },
    };
  };
  const checkPolicy = (p: AutoDeletePolicy) =>
    !Number.isInteger(p.min_age_days) || p.min_age_days < 1 || p.min_age_days > 3650
      ? problem(422, "The posts are kept for 1 to 3650 days")
      : undefined;

  // ---- Imports -----------------------------------------------------------------------------------------------------

  let importCount = 0;
  const imports: (DataImport & { startedAt: number })[] = settings.imports.map((i) => {
    importCount += 1;
    return {
      ...i,
      id: idAt(started, 120_000 + importCount),
      created_at: at(i.created_at),
      startedAt: Date.parse(at(i.created_at)),
    };
  });
  /** An import as it reads now: one started here finishes after a moment. */
  const importNow = ({ startedAt, ...i }: DataImport & { startedAt: number }): DataImport => {
    if (i.state === "finished") return i;
    const done = Date.now() - startedAt >= importSeconds * 1000;
    return done ? { ...i, state: "finished", processed: i.total, failed: Math.floor(i.total / 20) } : i;
  };
  const valid: Record<Schema<"DataImportKind">, RegExp> = {
    following: /^@?[\w.+-]+(@[\w-]+(\.[\w-]+)+)?(,.*)?$/,
    blocks: /^@?[\w.+-]+(@[\w-]+(\.[\w-]+)+)?(,.*)?$/,
    mutes: /^@?[\w.+-]+(@[\w-]+(\.[\w-]+)+)?(,.*)?$/,
    domain_mutes: /^[\w-]+(\.[\w-]+)+$/,
    followed_tags: /^#?[\p{L}\p{N}_]+$/u,
    bookmarks: /^https?:\/\/\S+$/,
  };
  const previewOf = (request: Schema<"DataImportRequest">): Schema<"DataImportPreview"> => {
    const lines = request.text.split(/\r?\n/).map((l) => l.trim());
    const skipped: Schema<"DataImportPreview">["skipped"] = [];
    const seen = new Set<string>();
    lines.forEach((value, i) => {
      // The first line of an export names its column; blank lines are no entries.
      if (value === "" || (i === 0 && /address|domain|hashtag|url|account/i.test(value))) return;
      const line = i + 1;
      if (!valid[request.kind].test(value)) skipped.push({ line, value, reason: "invalid" });
      else if (seen.has(value.toLowerCase())) skipped.push({ line, value, reason: "duplicate" });
      else seen.add(value.toLowerCase());
    });
    const entries = seen.size;
    return {
      entries,
      present: Math.floor(entries / 4),
      removals: request.mode === "overwrite" ? 5 : 0,
      skipped,
    };
  };

  // ---- Aliases and the move ----------------------------------------------------------------------------------------

  let aliasCount = 0;
  const handleOf = (acct: string) => acct.replace(/^@/, "");
  const aliasOf = (acct: string, createdAt: number): AccountAlias => {
    aliasCount += 1;
    const [user = "", domain = ""] = handleOf(acct).split("@");
    return {
      uri: `https://${domain}/users/${user}`,
      acct: handleOf(acct),
      created_at: new Date(createdAt - aliasCount * day).toISOString(),
    };
  };
  const aliases: AccountAlias[] = settings.aliases.map((a) => aliasOf(a, started));
  const aliasLimit = 5;
  let moveTarget: string | undefined;
  const confirmed = (p: {
    password?: string | undefined;
    passkey?: unknown;
    external_confirmation?: string | undefined;
  }) => p.passkey !== undefined || p.external_confirmation !== undefined || p.password === env.password;
  const wrongProof = () => refused("The confirmation does not hold", "reauthentication-failed");

  // ---- Deactivation ------------------------------------------------------------------------------------------------

  const deletionDue = () => new Date(Date.now() + 30 * day).toISOString();
  const refusal = settings.deactivationRefusal;

  // ---- Moderation --------------------------------------------------------------------------------------------------

  let appealCount = 0;
  const actions: ModerationAction[] = settings.moderation.map((a, i) => ({
    ...a,
    id: idAt(started, 130_000 + i),
    issued_at: at(a.issued_at),
    ...(a.expires_at === undefined ? {} : { expires_at: at(a.expires_at) }),
    ...(a.lifted_at === undefined ? {} : { lifted_at: at(a.lifted_at) }),
    ...(a.appeal_deadline === undefined ? {} : { appeal_deadline: at(a.appeal_deadline) }),
    ...(a.appeal === undefined
      ? {}
      : {
          appeal: {
            ...a.appeal,
            id: idAt(started, 140_000 + i),
            submitted_at: at(a.appeal.submitted_at),
            ...(a.appeal.decided_at === undefined ? {} : { decided_at: at(a.appeal.decided_at) }),
          },
        }),
  }));
  const hookAppeals = new Map<string, OwnHookAppeal>();

  const pageOf = <T extends { id: string }>(items: readonly T[], query: PageQuery) => page(items, query).body;

  const handlers: Handlers = {
    "GET /filters": () => ({ status: 200, body: { items: filters.map(nowFilter) } }),
    "POST /filters": ({ body }) => {
      if (settings.filterSave === "waits") return new Promise<never>(() => undefined);
      const request = trimmed(body);
      const refusal_ = checkFilter(request, filters);
      if (refusal_ !== undefined) return refusal_;
      if (filters.length >= filterLimits.filters)
        return refused("You have as many filters as you may", "too-many-keyword-filters", 422);
      const made = makeFilter(request, new Date().toISOString());
      filters.push(made);
      return { status: 201, body: nowFilter(made) };
    },
    "GET /filters/{id}": ({ params }) => {
      const found = filters.find((f) => f.id === params.id);
      return found === undefined ? notFound() : { status: 200, body: nowFilter(found) };
    },
    "PATCH /filters/{id}": ({ params, body }) => {
      if (settings.filterSave === "waits") return new Promise<never>(() => undefined);
      const at_ = filters.findIndex((f) => f.id === params.id);
      const old = filters[at_];
      if (old === undefined) return notFound();
      const request = trimmed(body);
      const refusal_ = checkFilter(
        request,
        filters.filter((f) => f.id !== old.id),
      );
      if (refusal_ !== undefined) return refusal_;
      const { expires_at, ...rest } = request;
      const saved: KeywordFilter = {
        ...rest,
        id: old.id,
        ...(expires_at === undefined ? {} : { expires_at }),
        expired: expires_at !== undefined && Date.parse(expires_at) <= Date.now(),
        created_at: old.created_at,
      };
      filters[at_] = saved;
      return { status: 200, body: saved };
    },
    "DELETE /filters/{id}": ({ params }) => {
      const at_ = filters.findIndex((f) => f.id === params.id);
      if (at_ < 0) return notFound();
      filters.splice(at_, 1);
      return { status: 204 };
    },

    "GET /preferences/badges": () => ({ status: 200, body: { items: badges } }),
    "PUT /preferences/badges/{role_id}": ({ params, body }) => {
      const at_ = badges.findIndex((b) => b.role_id === params.role_id);
      const old = badges[at_];
      if (old === undefined) return notFound();
      const saved = { ...old, displayed: body.displayed };
      badges[at_] = saved;
      return { status: 200, body: saved };
    },

    "GET /featured_tags": () => ({ status: 200, body: { items: featuredBody() } }),
    "PUT /featured_tags": ({ body }) => {
      // Each name with or without `#`, once; at most 10; the case of the first writing stays.
      const names: string[] = [];
      for (const raw of body.names) {
        const name = raw.replace(/^#/, "");
        if (!/^[\p{L}\p{M}\p{N}_]+$/u.test(name)) return problem(422, "Not a hashtag");
        if (!names.some((n) => n.toLowerCase() === name.toLowerCase())) names.push(name);
      }
      if (names.length > 10) return refused("You may feature 10 hashtags", "too-many-featured-tags", 422);
      featuredNames.splice(0, featuredNames.length, ...names);
      return { status: 200, body: { items: featuredBody() } };
    },

    "GET /preferences/notifications": () => ({ status: 200, body: { rows: rows() } }),
    "PATCH /preferences/notifications": ({ body }) => {
      for (const c of body.changes) {
        const row = seeded.find((r) => r.group === c.group);
        if (row === undefined) return problem(422, "The user does not get this row");
        if (row[c.channel] === undefined) return problem(422, "The row has no such channel");
        if (row[c.channel] === "fixed") return problem(422, "The channel is always on");
      }
      for (const c of body.changes) cells.set(`${c.group}/${c.channel}`, c.enabled);
      return { status: 200, body: { rows: rows() } };
    },
    "GET /preferences/notification_policy": () => ({ status: 200, body: policy }),
    "PATCH /preferences/notification_policy": ({ body }) => {
      policy = {
        not_following: body.not_following ?? policy.not_following,
        not_follower: body.not_follower ?? policy.not_follower,
        new_account: body.new_account ?? policy.new_account,
        unsolicited_private_mention: body.unsolicited_private_mention ?? policy.unsolicited_private_mention,
        limited_account: body.limited_account ?? policy.limited_account,
      };
      return { status: 200, body: policy };
    },

    "GET /preferences/auto_delete": () => ({ status: 200, body: settingsOf(autoDelete) }),
    "PUT /preferences/auto_delete": ({ body }) => {
      const refusal_ = checkPolicy(body);
      if (refusal_ !== undefined) return refusal_;
      // `keep_articles` left out keeps the saved value.
      autoDelete = { ...body, keep_articles: body.keep_articles ?? autoDelete.keep_articles ?? true };
      return { status: 200, body: settingsOf(autoDelete) };
    },
    "POST /preferences/auto_delete/preview": ({ body }) => {
      const refusal_ = checkPolicy(body);
      if (refusal_ !== undefined) return refusal_;
      const count = estimate(body.min_age_days);
      return {
        status: 200,
        body: { count: Math.min(count, 1000), more: count > 1000, examples: env.ownStatuses().slice(-3).toReversed() },
      };
    },

    "GET /imports": () => ({ status: 200, body: { items: imports.map(importNow) } }),
    "POST /imports/preview": ({ body }) => ({ status: 200, body: previewOf(body) }),
    "POST /imports": ({ body }) => {
      if (imports.some((i) => importNow(i).state === "in_progress"))
        return refused("An import is already being carried out", "import-running", 409);
      const preview = previewOf(body);
      importCount += 1;
      const now = Date.now();
      const made = {
        id: idAt(now, 120_000 + importCount),
        kind: body.kind,
        mode: body.mode,
        state: "in_progress" as const,
        created_at: new Date(now).toISOString(),
        total: preview.entries + preview.removals,
        processed: 0,
        failed: 0,
        startedAt: now,
      };
      imports.unshift(made);
      return { status: 202, body: importNow(made) };
    },

    "GET /accounts/aliases": () => ({ status: 200, body: { items: aliases } }),
    "POST /accounts/aliases": ({ body }) => {
      const handle = handleOf(body.acct.trim());
      const url = /^https:\/\/([^/]+)\/(?:users\/|@)([^/]+)$/.exec(handle);
      const acct = url === null ? handle : `${url[2]}@${url[1]}`;
      if (!/^[\w.-]+@[\w-]+(\.[\w-]+)+$/.test(acct))
        return refused("The account cannot be found", "account-not-found", 422);
      if (aliases.some((a) => a.acct === acct)) return { status: 200, body: { items: aliases } };
      if (aliases.length >= aliasLimit) return refused("You have as many aliases as you may", "too-many-aliases", 422);
      aliases.push(aliasOf(acct, Date.now()));
      return { status: 200, body: { items: aliases } };
    },
    "DELETE /accounts/aliases": ({ query }) => {
      const at_ = aliases.findIndex((a) => a.uri === query.get("uri"));
      if (at_ >= 0) aliases.splice(at_, 1);
      return { status: 200, body: { items: aliases } };
    },
    "POST /accounts/move": ({ body }) => {
      if (!confirmed(body)) return wrongProof();
      if (!env.accountExists(body.target_account_id)) return notFound();
      if (aliases.length === 0)
        return refused("The target does not have this account as an alias", "move-target-without-alias", 422);
      moveTarget = body.target_account_id;
      return { status: 204 };
    },
    "DELETE /accounts/move": () => {
      if (moveTarget === undefined) return notFound();
      moveTarget = undefined;
      return { status: 204 };
    },

    "GET /accounts/deactivation": () => ({
      status: 200,
      body: refusal === undefined ? { deletion_due_at: deletionDue() } : { deletion_due_at: deletionDue(), refusal },
    }),
    "POST /accounts/deactivation": ({ body }) => {
      if (refusal !== undefined) return refused("The account cannot be deactivated", `deactivation-${refusal}`, 409);
      if (!confirmed(body)) return wrongProof();
      return { status: 200, body: { deletion_due_at: deletionDue() } };
    },
    "POST /accounts/delete": ({ body }) => {
      if (!confirmed(body)) return wrongProof();
      return { status: 204 };
    },

    "GET /moderation/actions": ({ query }) => {
      const shown = query.get("active") === "true" ? actions.filter((a) => a.state === "active") : actions;
      return { status: 200, body: pageOf(shown, query) };
    },
    "POST /moderation/actions/{id}/appeal": ({ params, body }): Answer<"POST /moderation/actions/{id}/appeal"> => {
      const at_ = actions.findIndex((a) => a.id === params.id);
      const old = actions[at_];
      if (old === undefined) return notFound();
      if (!old.appealable) return refused("The action cannot be appealed now", "not-appealable", 409);
      const text = body.text.trim();
      if (text === "") return problem(422, "Say why the action should be reconsidered");
      appealCount += 1;
      const saved: ModerationAction = {
        ...old,
        appealable: false,
        appeal: {
          id: idAt(Date.now(), 150_000 + appealCount),
          text,
          state: "pending",
          submitted_at: new Date().toISOString(),
        },
      };
      actions[at_] = saved;
      return { status: 200, body: saved };
    },
    "GET /hook_decisions/{id}/appeal": ({ params }) => {
      const found = hookAppeals.get(params.id);
      return found === undefined ? notFound() : { status: 200, body: found };
    },
    "POST /hook_decisions/{id}/appeal": ({ params, body }) => {
      if (hookAppeals.has(params.id)) return refused("The decision is appealed already", "already-appealed", 409);
      if (body.status_text.trim() === "") return problem(422, "Say what the post said");
      appealCount += 1;
      const made: OwnHookAppeal = {
        id: idAt(Date.now(), 150_000 + appealCount),
        hook_decision_id: params.id,
        state: "pending",
        submitted_at: new Date().toISOString(),
      };
      hookAppeals.set(params.id, made);
      return { status: 200, body: made };
    },
  };

  const shownBadges = () =>
    badges.filter((b) => b.displayed).map(({ role_id, label, color }) => ({ role_id, label, color }));
  const filterId = (index: number) => filters[index]?.id;
  return { handlers, judged, featured: featuredBody, shownBadges, filterId };
}
