// SPDX-License-Identifier: AGPL-3.0-only

// The fake's reports (FR-701, FR-709–FR-712, FR-727, ADR-0058): the moderators' queue as a holder of `manage_reports`
// reads it under /admin/reports, and what they do to a report (take it, leave it to nobody, write a note, resolve it,
// reopen it, forward a remote one once, translate its comment and its posts). The queue is the fake's own, since the
// seed files no report: a spam account reported with a post its author deleted since, a remote photographer's repost
// assigned to the scenario's moderator with a note, a report another server sent, a voice chat room that ended and one
// still open, an event, one an external review filed and resolved, and one forwarded when filed and resolved. A report filed on the fake
// (`file`, POST /reports) joins it. The instance's rules are the fake's too, and one report cites a rule retired since.
// What is translated reads "[ja] …" as the server's fake provider does, only while `translation.reports` lets the
// server send a report's text; its comment counts as a private post.

import { day, idAt, minute, notFound, page, problem } from "./common";
import { serverHtml } from "./html";
import type { Answer, Handlers, Schema } from "./spec";

type Report = Schema<"AdminReportDetail">;
type Account = Schema<"Account">;
type Status = Schema<"Status">;

/**
 * What the queue holds when the fake starts: the fake's reports (`queue`, by default), or none; `crowded`, kai reported
 * 24 times more as well, a day apart before them, so that the queue and kai's reports run past a page.
 */
export type FakeReportOptions = "queue" | "none" | "crowded";

/** The reports of `crowded`: kai's replies reported as spam by sam and layla in turn, a day apart. */
const crowd: readonly Plan[] = Array.from({ length: 24 }, (_, i) => ({
  key: `crowd-${String(i + 1)}`,
  reporter: i % 2 === 0 ? "sam" : "layla",
  target: "kai",
  category: "spam" as const,
  comment: `Another reply with the same link, number ${String(i + 1)}.`,
  minutesAgo: (i + 2) * 24 * 60,
}));

/** What the reports' handlers take from the fake. */
export type ReportContext = {
  readonly started: number;
  readonly options: FakeReportOptions | undefined;
  /** The reader's username; their permissions decide whether they read the queue. */
  readonly viewer: string;
  readonly permissions: () => readonly Schema<"Permission">[];
  readonly accountOf: (username: string) => Account;
  /** Whether the fake has the person of `username`, and the seed's post of `key`: a seed of a test may have neither. */
  readonly known: (username: string) => boolean;
  readonly knownPost: (key: string) => boolean;
  readonly accountId: (username: string) => string;
  /** The account of the ID, if it is a person of the fake. */
  readonly accountById: (id: string) => Account | undefined;
  /** The status of the seed's post of `key`, as anyone reads it. */
  readonly statusOf: (key: string) => Status;
  /** The status of the ID, as anyone reads it, if the fake has it. */
  readonly statusById: (id: string) => Status | undefined;
  /** The ID of the fake's event of `key`. */
  readonly eventId: (key: string) => string;
  /** The server sends a report's text to its provider, which answers (`[ja] …`), and receives private posts. */
  readonly translates: boolean;
  /** The domain the fake answers as. */
  readonly host: string;
};

/** What a test does with the fake's reports. */
export type FakeReports = {
  /** The ID of the fake's report of `key` (deals, photo, flag, room, live, event, review, copied), or "none". */
  readonly id: (key: string) => string;
  /** The reports that are open now. */
  readonly openCount: () => number;
  /** When the open report that has waited longest was filed; undefined while none is open. */
  readonly oldestOpen: () => string | undefined;
  /** Files a report of `reporter`'s, as POST /reports does, and returns its ID ("none" for an account it lacks). */
  readonly file: (reporter: string, request: Schema<"CreateReportRequest">) => string;
  /** The newest reports of the account of `accountId`, as the queue lists them, newest first. */
  readonly ofAccount: (accountId: string) => Schema<"AdminReport">[];
  /** Whether the report of `id` is of the account of `accountId`; resolves it as `actioned` when it is open. */
  readonly actioned: (id: string, accountId: string) => boolean;
  /** Ends the open voice chat room of `id` that a report points at, as a moderator; false for none open. */
  readonly endRoom: (id: string, note: string | undefined) => boolean;
  /** Cancels the event of `id` that a report points at, as a moderator. */
  readonly cancelEvent: (id: string, note: string | undefined) => "cancelled" | "closed" | "missing";
};

/** The rules of the fake's instance, in order; and one retired since a report cited it. */
export function fakeRules(started: number): { rules: Schema<"InstanceRule">[]; retired: string } {
  const id = (n: number) => idAt(started - 400 * day, 40_000 + n);
  return {
    rules: [
      { id: id(1), text: "Be kind to each other.", hint: "No insults, threats or pile-ons." },
      { id: id(2), text: "Credit the photographer when you share a photo." },
      { id: id(3), text: "Mark sensitive media as sensitive." },
      { id: id(4), text: "No advertising without the moderators' permission." },
    ],
    retired: id(5),
  };
}

/** A report of the fake's queue as it starts. */
type Plan = {
  key: string;
  /** Who filed it: a username, or `actor` for this server's instance actor (an external review). */
  reporter: string;
  target: string;
  category: Schema<"ReportCategory">;
  comment: string;
  /** The seed's posts it points at, by key, and those its author deleted since (the fake's own). */
  posts?: readonly string[];
  deleted?: readonly { text: string; minutesAgo: number }[];
  /** The rules it cites, by their place among the fake's rules (1-based), 0 for the retired one. */
  rules?: readonly number[];
  minutesAgo: number;
  /** Another server sent it (a `Flag`). */
  remote?: boolean;
  forwarded?: boolean;
  /** Who handles it: a username, or `viewer` for the reader. */
  assignee?: string;
  notes?: readonly { by?: string; body: string; minutesAgo: number }[];
  resolved?: { outcome: Schema<"AdminReportOutcome">; minutesAgo: number };
  /** The voice chat room it points at, which its target hosted: ended before the report, or still open. */
  room?: "ended" | "open";
  event?: string;
  review?: boolean;
};

const plans: readonly Plan[] = [
  {
    key: "deals",
    reporter: "sam",
    target: "quickdeals",
    category: "spam",
    comment:
      "Replies to every post about bread with the same discount link. Two of them were deleted before I could report them.",
    posts: ["quickdeals-spam"],
    deleted: [
      { text: "Flash sale on sourdough starters!!! https://deals.invalid/bread", minutesAgo: 30 },
      { text: "Last chance, 90% off https://deals.invalid/last", minutesAgo: 25 },
    ],
    minutesAgo: 12,
  },
  {
    key: "photo",
    reporter: "layla",
    target: "sora@photos.example",
    category: "violation",
    comment: "この写真は私が撮ったものです。クレジットなしで転載されています。",
    posts: ["sora-film"],
    rules: [2],
    minutesAgo: 40,
    assignee: "marco",
    notes: [{ by: "marco", body: "Asked layla for the original file to compare.", minutesAgo: 20 }],
  },
  {
    key: "flag",
    reporter: "kiri@far.example",
    target: "kai",
    category: "spam",
    comment: "Sent from far.example: the account sends the same direct message to our users, unasked.",
    posts: ["kai-hello"],
    minutesAgo: 3 * 60,
    remote: true,
  },
  {
    key: "room",
    reporter: "ren",
    target: "kai",
    category: "violation",
    comment: "The host removed me after I asked them to stop shouting at a guest.",
    rules: [1, 0],
    minutesAgo: 5 * 60,
    assignee: "viewer",
    room: "ended",
  },
  {
    key: "live",
    reporter: "sam",
    target: "ren",
    category: "violation",
    comment: "The host keeps playing a siren into the room and removes whoever asks them to stop.",
    rules: [1],
    minutesAgo: 8,
    room: "open",
  },
  {
    key: "event",
    reporter: "sam",
    target: "mio",
    category: "other",
    comment: "The list of books links to a shop that sells them instead of a swap.",
    minutesAgo: 26 * 60,
    event: "book-swap",
  },
  {
    key: "review",
    reporter: "actor",
    target: "quickdeals",
    category: "spam",
    comment: "Matched 4 links of a known spam campaign.",
    posts: ["quickdeals-spam"],
    minutesAgo: 2 * 24 * 60,
    review: true,
    assignee: "marco",
    notes: [{ by: "marco", body: "Same campaign as the account's earlier posts.", minutesAgo: 2 * 24 * 60 - 30 }],
    resolved: { outcome: "actioned", minutesAgo: 2 * 24 * 60 - 40 },
  },
  {
    key: "copied",
    reporter: "sam",
    target: "kiri@far.example",
    category: "legal",
    comment: "Shares scans of a whole magazine issue.",
    minutesAgo: 3 * 24 * 60,
    forwarded: true,
    notes: [
      { by: "hana", body: "far.example's moderators took the scans down.", minutesAgo: 3 * 24 * 60 - 120 },
      { body: "Checked with my admin report show.", minutesAgo: 3 * 24 * 60 - 100 },
    ],
    resolved: { outcome: "dismissed", minutesAgo: 3 * 24 * 60 - 90 },
  },
];

/** The handlers of the reports, and what a test does with them. */
export function reportHandlers(c: ReportContext): { handlers: Handlers; reports: FakeReports } {
  const iso = (ms: number) => new Date(ms).toISOString();
  const ago = (minutes: number) => c.started - minutes * minute;
  const { rules, retired } = fakeRules(c.started);
  const ruleId = (n: number) => (n === 0 ? retired : (rules[n - 1]?.id ?? retired));
  const remote = (username: string) => username.includes("@");
  /** This server's instance actor, which files the reports of an external review, made of a report's target. */
  const actorOf = (target: string): Account => ({
    ...c.accountOf(target),
    id: idAt(0, 900),
    username: c.host,
    acct: c.host,
    display_name: c.host,
    note: "",
    bot: true,
    fields: [],
  });
  let ids = 0;
  const reportId = (ms: number) => idAt(ms, 45_000 + (ids += 1));
  const keyed = new Map<string, string>();

  const voiceRoom = (filed: number, host: string, open: boolean): Schema<"AdminReportVoiceRoom"> => {
    const revision = idAt(filed - 60 * minute, 45_901);
    const opened = filed - 70 * minute;
    const kai = c.accountId(host);
    const others = (["sam", "ren"] as const).filter((u) => u !== host);
    if (open)
      return {
        id: idAt(opened, 45_920),
        reported_revision_id: revision,
        opened_at: iso(opened),
        revisions: [
          {
            id: revision,
            name: "Open mic night",
            description: "Bring a song.",
            hashtags: ["music"],
            visibility: "local",
            speakers: "everyone",
            revised_by_account_id: kai,
            revised_at: iso(opened),
          },
        ],
        participations: [
          { account_id: kai, joined_at: iso(opened) },
          ...others.map((u, i) => ({ account_id: c.accountId(u), joined_at: iso(opened + (i + 1) * 5 * minute) })),
        ],
        bans: [],
      };
    return {
      id: idAt(opened, 45_900),
      reported_revision_id: revision,
      opened_at: iso(opened),
      ended_at: iso(filed - 10 * minute),
      end_reason: "host",
      ended_by_account_id: kai,
      revisions: [
        {
          id: idAt(opened, 45_902),
          name: "Late night radio",
          description: "Music and chat until we fall asleep.",
          hashtags: ["radio"],
          visibility: "local",
          speakers: "manual",
          revised_by_account_id: kai,
          revised_at: iso(opened),
        },
        {
          id: revision,
          name: "Late night radio: shouting allowed",
          description: "Anything goes after midnight.",
          hashtags: ["radio"],
          visibility: "local",
          speakers: "everyone",
          revised_by_account_id: kai,
          revised_at: iso(filed - 60 * minute),
        },
      ],
      participations: [
        { account_id: kai, joined_at: iso(opened), left_at: iso(filed - 10 * minute), leave_reason: "room_ended" },
        {
          account_id: c.accountId("sam"),
          joined_at: iso(opened + 5 * minute),
          left_at: iso(filed - 30 * minute),
          leave_reason: "left",
        },
        {
          account_id: c.accountId("ren"),
          joined_at: iso(opened + 10 * minute),
          left_at: iso(filed - 15 * minute),
          leave_reason: "removed",
        },
      ],
      bans: [{ account_id: c.accountId("ren"), banned_by_account_id: kai, banned_at: iso(filed - 15 * minute) }],
    };
  };

  const eventOf = (key: string, filed: number, organizer: string): Schema<"AdminReportEvent"> => {
    const id = c.eventId(key);
    const by = c.accountId(organizer);
    const created = filed - 3 * day;
    const seen = idAt(filed - day, 45_911);
    const starts = c.started + 7 * day;
    const revision = (at: number, rid: string, description: string, list: string): Schema<"AdminEventRevision"> => ({
      id: rid,
      title: "Book swap",
      description,
      visibility: "public",
      time_zone: "Europe/Paris",
      place_disclosure: "approved",
      capacity: 0,
      places: [
        { kind: "physical", name: "Café Lumière", address: "12 rue des Lilas" },
        { kind: "url", name: "The list of books", url: list },
      ],
      revised_by_account_id: by,
      revised_at: iso(at),
    });
    return {
      id,
      organizer_account_id: by,
      created_at: iso(created),
      state: "scheduled",
      starts_at: iso(starts),
      ends_at: iso(starts + 2 * 60 * minute),
      reported_revision_id: seen,
      revisions: [
        revision(created, idAt(created, 45_910), "Bring a book, take a book.", "https://books.example/swap"),
        revision(filed - day, seen, "Bring a book, take a book. Or buy one.", "https://shop.example/books"),
        revision(
          filed + 60 * minute,
          idAt(filed + 60 * minute, 45_912),
          "Bring a book, take a book.",
          "https://books.example/swap",
        ),
      ],
    };
  };

  const deletedStatus = (target: string, text: string, at: number, n: number): Status => ({
    ...c.statusOf("quickdeals-spam"),
    id: idAt(at, 45_800 + n),
    account: c.accountOf(target),
    content: serverHtml(text, c.host),
    created_at: iso(at),
    replies_count: 0,
    boosts_count: 0,
    favourites_count: 0,
    reactions: [],
  });

  /** A report as the server holds it, from a plan. */
  const reportOf = (plan: Plan): Report => {
    const filed = ago(plan.minutesAgo);
    const id = reportId(filed);
    keyed.set(plan.key, id);
    const statuses: Schema<"AdminReportStatus">[] = [
      ...(plan.posts ?? []).map((key) => ({ status: c.statusOf(key) })),
      ...(plan.deleted ?? []).map((d, i) => ({
        status: deletedStatus(plan.target, d.text, ago(d.minutesAgo + 60), i),
        deleted_at: iso(ago(d.minutesAgo)),
      })),
    ];
    const reporter = plan.reporter === "actor" ? actorOf(plan.target) : c.accountOf(plan.reporter);
    const assignee = plan.assignee === "viewer" ? c.viewer : plan.assignee;
    const forwarded = plan.forwarded === true;
    const forwardable = !plan.remote && remote(plan.target) && plan.room === undefined && plan.event === undefined;
    return {
      id,
      ...(plan.remote === true ? { uri: `https://far.example/flags/${id}` } : {}),
      category: plan.category,
      comment: plan.comment,
      forwarded,
      forwardable: forwardable && !forwarded,
      filed_at: iso(filed),
      state: plan.resolved === undefined ? "open" : "resolved",
      ...(plan.resolved === undefined ? {} : { outcome: plan.resolved.outcome }),
      state_changed_at: iso(plan.resolved === undefined ? filed : ago(plan.resolved.minutesAgo)),
      ...(assignee === undefined ? {} : { assignee_account_id: c.accountId(assignee) }),
      reporter_account_id: reporter.id,
      reporter_account: reporter,
      target_account_id: c.accountId(plan.target),
      target_account: c.accountOf(plan.target),
      status_ids: statuses.map((s) => s.status.id),
      ...(plan.room === undefined ? {} : { voice_room_id: voiceRoom(filed, plan.target, plan.room === "open").id }),
      ...(plan.event === undefined ? {} : { event_id: c.eventId(plan.event) }),
      ...(plan.review === true
        ? { hook_origin: { hook: "local.status", decision_id: idAt(filed, 45_700), reason: "spam" } }
        : {}),
      rule_ids: (plan.rules ?? []).map(ruleId),
      notes: (plan.notes ?? []).map((n, i) => ({
        id: idAt(ago(n.minutesAgo), 45_600 + i),
        ...(n.by === undefined ? {} : { author_account_id: c.accountId(n.by) }),
        body: n.body,
        created_at: iso(ago(n.minutesAgo)),
      })),
      target_history: { reports_count: 0, actions_count: plan.target === "quickdeals" ? 1 : 0 },
      statuses,
      ...(plan.room === undefined ? {} : { voice_room: voiceRoom(filed, plan.target, plan.room === "open") }),
      ...(plan.event === undefined ? {} : { event: eventOf(plan.event, filed, plan.target) }),
    };
  };

  // The queue, newest first, each target's other reports counted.
  // A plan whose people or posts a test's seed lacks is no report of it.
  const peopleOf = (plan: Plan) => [
    plan.target,
    ...(plan.reporter === "actor" ? [] : [plan.reporter]),
    ...(plan.assignee === undefined || plan.assignee === "viewer" ? [] : [plan.assignee]),
    ...(plan.notes ?? []).flatMap((n) => (n.by === undefined ? [] : [n.by])),
  ];
  const whole = (plan: Plan) =>
    peopleOf(plan).every(c.known) &&
    (plan.posts ?? []).every(c.knownPost) &&
    (plan.event === undefined || c.eventId(plan.event) !== "none");
  const planned = c.options === "none" ? [] : c.options === "crowded" ? [...plans, ...crowd] : plans;
  const reports: Report[] = planned
    .filter(whole)
    .map(reportOf)
    .toSorted((a, b) => b.filed_at.localeCompare(a.filed_at));
  for (const r of reports)
    r.target_history.reports_count = reports.filter(
      (o) => o.target_account_id === r.target_account_id && o.id !== r.id,
    ).length;

  const allowed = () => c.permissions().includes("manage_reports");
  const forbidden = () => problem(403, "Forbidden", { detail: "This needs the manage_reports permission." });
  const found = (id: string) => reports.find((r) => r.id === id);
  /** The report as the list gives it: without its notes, its posts, and what it points at. */
  const listed = ({
    notes: _notes,
    target_history: _history,
    statuses: _statuses,
    voice_room: _room,
    event: _event,
    ...r
  }: Report): Schema<"AdminReport"> => r;

  /** Answers with what `change` makes of the report `id`, as the reader may. */
  const changed = (
    id: string,
    change: (r: Report) => Answer<"POST /admin/reports/{id}/resolve"> | undefined,
  ): Answer<"POST /admin/reports/{id}/resolve"> => {
    if (!allowed()) return forbidden();
    const r = found(id);
    if (r === undefined) return notFound();
    return change(r) ?? { status: 200, body: structuredClone(r) };
  };

  const queue = (query: {
    get: (name: "state" | "category" | "assignee" | "target_account_id" | "order") => string | null;
  }) => {
    const state = query.get("state");
    const category = query.get("category");
    const assignee = query.get("assignee");
    const target = query.get("target_account_id");
    const me = c.accountId(c.viewer);
    const kept = reports.filter(
      (r) =>
        (state === null || r.state === state) &&
        (category === null || r.category === category) &&
        (target === null || r.target_account_id === target) &&
        (assignee === null ||
          (assignee === "none"
            ? r.assignee_account_id === undefined
            : r.assignee_account_id === (assignee === "me" ? me : assignee))),
    );
    return query.get("order") === "oldest" ? kept.toReversed() : kept;
  };

  /** The text into `language`, as the server's fake provider writes it; a refusal while the server sends none. */
  const translated = (visibility: string) =>
    c.translates && visibility !== "direct"
      ? undefined
      : problem(422, "No translation provider may receive this text", {
          type: "/api/v1/problems/translation-unavailable",
        });

  const handlers: Handlers = {
    "GET /admin/reports": ({ query }) => {
      if (!allowed()) return forbidden();
      const assignee = query.get("assignee");
      if (assignee !== null && assignee !== "me" && assignee !== "none" && c.accountById(assignee) === undefined)
        return problem(400, "Bad Request", { detail: "assignee is me, none, or a local account's ID." });
      const { body } = page(queue(query), query);
      return { status: 200, body: { ...body, items: body.items.map(listed) } };
    },
    "GET /admin/reports/{id}": ({ params }) => changed(params.id, () => undefined),
    "POST /admin/reports/{id}/assign_to_self": ({ params }) =>
      changed(params.id, (r) => {
        r.assignee_account_id = c.accountId(c.viewer);
        return undefined;
      }),
    "POST /admin/reports/{id}/unassign": ({ params }) =>
      changed(params.id, (r) => {
        delete r.assignee_account_id;
        return undefined;
      }),
    "POST /admin/reports/{id}/notes": ({ params, body }) =>
      changed(params.id, (r) => {
        if (body.body.trim() === "")
          return problem(422, "Unprocessable Content", { detail: "The note may not be blank.", field: "body" });
        r.notes.push({
          id: idAt(Date.now(), 45_650 + r.notes.length),
          author_account_id: c.accountId(c.viewer),
          body: body.body,
          created_at: iso(Date.now()),
        });
        return undefined;
      }),
    "POST /admin/reports/{id}/resolve": ({ params, body }) =>
      changed(params.id, (r) => {
        if (r.state === "resolved") return undefined;
        r.state = "resolved";
        r.outcome = body.outcome;
        r.state_changed_at = iso(Date.now());
        return undefined;
      }),
    "POST /admin/reports/{id}/reopen": ({ params }) =>
      changed(params.id, (r) => {
        if (r.state === "open") return problem(409, "Conflict", { detail: "The report is open." });
        r.state = "open";
        delete r.outcome;
        r.state_changed_at = iso(Date.now());
        return undefined;
      }),
    "POST /admin/reports/{id}/forward": ({ params, body }) =>
      changed(params.id, (r) => {
        if ((body?.comment ?? "").length > 1000)
          return problem(422, "Unprocessable Content", { detail: "The text is too long.", field: "comment" });
        if (r.forwarded) return undefined;
        if (!r.forwardable)
          return problem(409, "The report cannot be forwarded", { type: "/api/v1/problems/report-not-forwardable" });
        r.forwarded = true;
        r.forwardable = false;
        return undefined;
      }),
    "POST /admin/reports/{id}/comment/translation": ({ params, body }) => {
      if (!allowed()) return forbidden();
      const r = found(params.id);
      if (r === undefined) return notFound();
      const refusal = translated("private");
      if (refusal !== undefined) return refusal;
      return {
        status: 200,
        body: {
          language: body.target_language,
          source_language: /[぀-ヿ一-鿿]/.test(r.comment) ? "ja" : "en",
          comment: `[${body.target_language}] ${r.comment}`,
          provider: { id: "fake", name: "Demo Translate" },
        },
      };
    },
    "POST /admin/reports/{id}/statuses/{status_id}/translation": ({ params, body }) => {
      if (!allowed()) return forbidden();
      const s = found(params.id)?.statuses.find((x) => x.status.id === params.status_id)?.status;
      if (s === undefined) return notFound();
      const refusal = translated(s.visibility);
      if (refusal !== undefined) return refusal;
      const into = (html: string) => html.replace(/^<p>/, `<p>[${body.target_language}] `);
      return {
        status: 200,
        body: {
          language: body.target_language,
          source_language: s.language,
          content: into(s.content),
          spoiler_text: s.spoiler_text === "" ? "" : `[${body.target_language}] ${s.spoiler_text}`,
          media_attachments: [],
          poll_options: [],
          provider: { id: "fake", name: "Demo Translate" },
        },
      };
    },
  };

  /** Files `reporter`'s report, as the server files it, and returns it as the reporter reads it. */
  const filed = (reporter: string, request: Schema<"CreateReportRequest">): Schema<"Report"> | undefined => {
    const target = c.accountById(request.account_id);
    if (target === undefined) return undefined;
    const now = Date.now();
    const id = reportId(now);
    const by = c.accountOf(reporter);
    const statuses = (request.status_ids ?? []).flatMap((sid) => {
      const status = c.statusById(sid);
      return status === undefined ? [] : [{ status }];
    });
    const away = remote(target.acct);
    const pointsAtPlace = request.voice_room_id !== undefined || request.event_id !== undefined;
    const forwarded = request.forward && away && !pointsAtPlace;
    reports.unshift({
      id,
      category: request.category,
      comment: request.comment ?? "",
      forwarded,
      forwardable: !forwarded && away && !pointsAtPlace,
      filed_at: iso(now),
      state: "open",
      state_changed_at: iso(now),
      reporter_account_id: by.id,
      reporter_account: by,
      target_account_id: target.id,
      target_account: target,
      status_ids: statuses.map((x) => x.status.id),
      ...(request.voice_room_id === undefined ? {} : { voice_room_id: request.voice_room_id }),
      ...(request.event_id === undefined ? {} : { event_id: request.event_id }),
      rule_ids: request.category === "violation" ? [...(request.rule_ids ?? [])] : [],
      notes: [],
      target_history: {
        reports_count: reports.filter((r) => r.target_account_id === target.id).length,
        actions_count: 0,
      },
      statuses,
    });
    return {
      id,
      category: request.category,
      comment: request.comment ?? "",
      forwarded,
      filed_at: iso(now),
      target_account: target,
      status_ids: statuses.map((x) => x.status.id),
      rule_ids: request.category === "violation" ? [...(request.rule_ids ?? [])] : [],
      ...(request.voice_room_id === undefined ? {} : { voice_room_id: request.voice_room_id }),
      ...(request.event_id === undefined ? {} : { event_id: request.event_id }),
      state: "open",
    };
  };
  handlers["POST /reports"] = ({ body }) => {
    const report = filed(c.viewer, body);
    return report === undefined
      ? problem(422, "Unprocessable Content", { field: "account_id" })
      : { status: 201, body: report };
  };

  const open = () => reports.filter((r) => r.state === "open");
  return {
    handlers,
    reports: {
      id: (key) => keyed.get(key) ?? "none",
      openCount: () => open().length,
      oldestOpen: () => open().at(-1)?.filed_at,
      file: (reporter, request) => filed(reporter, request)?.id ?? "none",
      ofAccount: (accountId) =>
        reports
          .filter((r) => r.target_account_id === accountId)
          .slice(0, 20)
          .map(listed),
      actioned: (id, accountId) => {
        const r = found(id);
        if (r?.target_account_id !== accountId) return false;
        if (r.state === "open") {
          r.state = "resolved";
          r.outcome = "actioned";
          r.state_changed_at = iso(Date.now());
        }
        return true;
      },
      endRoom: (id, note) => {
        const rooms = reports.flatMap((r) => (r.voice_room?.id === id ? [r.voice_room] : []));
        if (rooms.length === 0 || rooms.some((room) => room.ended_at !== undefined)) return false;
        const at = iso(Date.now());
        for (const room of rooms) {
          room.ended_at = at;
          room.end_reason = "moderator";
          room.ended_by_account_id = c.accountId(c.viewer);
          if (note !== undefined && note !== "") room.end_note = note;
          for (const p of room.participations)
            if (p.left_at === undefined) {
              p.left_at = at;
              p.leave_reason = "room_ended";
            }
        }
        return true;
      },
      cancelEvent: (id, note) => {
        const events = reports.flatMap((r) => (r.event?.id === id ? [r.event] : []));
        if (events.length === 0) return "missing";
        if (events.some((e) => e.state === "canceled")) return "closed";
        for (const e of events) {
          e.state = "canceled";
          e.cancellation = {
            reason: "moderator",
            canceled_by_account_id: c.accountId(c.viewer),
            ...(note === undefined || note === "" ? {} : { note }),
            canceled_at: iso(Date.now()),
          };
        }
        return "cancelled";
      },
    },
  };
}
