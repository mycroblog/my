// SPDX-License-Identifier: AGPL-3.0-only

// The fake's operations (ADR-0004, ADR-0062, ADR-0103 §8, ADR-0167 §3), as the administration reads them: the job
// queue of the database (a job running, three queued, one waiting to retry after its failures, and three failed for
// good, each with the runs that did not succeed), the failed one of `key` run again as the server does (a new count of
// attempts, queued at once); the error groups the server and the web client reported (a panic of a route and an error
// of the web client, open; one resolved on an earlier version; one muted), each with its latest occurrence, its hours
// and its history, resolved, muted and unmuted; the UX measurements of a window, which the server holds only while the
// instance setting `ux_measurement` is on; and the rules of the link card crawler, three of them, set and removed.
// `none` starts every list empty.

import { idAt, minute, notFound, page, problem } from "./common";
import type { Handlers, Schema } from "./spec";

type Job = Schema<"AdminJob">;
type Attempt = Schema<"AdminJobAttempt">;
type Group = Schema<"ErrorGroupDetail">;

/**
 * What the operations hold when the fake starts: the fake's own (by default), or nothing; `crowded`, the fake's own
 * with 45 more rules of the link card crawler (`site01.example` to `site45.example`) and the first error group's
 * occurrences over 30 hours, so that those lists run past what a screen shows at once.
 */
export type FakeOperationsOptions = "fake" | "none" | "crowded";

/** What the operations' handlers take from the fake. */
export type OperationsContext = {
  readonly started: number;
  readonly options: FakeOperationsOptions | undefined;
  readonly permissions: () => readonly Schema<"Permission">[];
  /** The value of an instance setting now (admin-settings.ts). */
  readonly setting: (key: Schema<"InstanceSettingKey">) => string;
};

/** What a test or a story does with the operations of the fake. */
export type FakeOperations = {
  /** The ID of the fake's job or error group of `key` (`job:dead-email`, `error:panic`), or "none". */
  readonly id: (key: string) => string;
  /** The error groups that are open. */
  readonly openErrorGroups: () => number;
};

/** A job of the fake as it starts: its key, and what the server holds of it. */
type JobPlan = {
  key: string;
  type: string;
  state: Schema<"AdminJobState">;
  payload: unknown;
  attempts: number;
  /** Minutes from the start: when it was created, when it is due (pending), or when it failed (dead). */
  created: number;
  at: number;
  worker?: string;
  idempotency?: string;
  ordering?: string;
  /** Why each run failed, the first first; the last is the job's last error. */
  errors?: readonly string[];
};

const jobPlans: readonly JobPlan[] = [
  {
    key: "running",
    type: "federation.deliver",
    state: "running",
    payload: { delivery_id: "01J9Z3W9PN5ZS3H8Q4D3E7GZ1K", inbox: "https://mastodon.example/inbox" },
    attempts: 1,
    created: -1,
    at: -1,
    worker: "worker-7f3a",
    ordering: "federation.deliver:mastodon.example",
  },
  {
    key: "queued",
    type: "notification.post_subscribers",
    state: "queued",
    payload: { status_id: "01J9Z3X0D7W2A1M4QF3T0Y8V6B" },
    attempts: 0,
    created: -2,
    at: -2,
    idempotency: "notification.post_subscribers:01J9Z3X0D7W2A1M4QF3T0Y8V6B",
  },
  {
    key: "queued-search",
    type: "search.index_status",
    state: "queued",
    payload: { status_id: "01J9Z3X0D7W2A1M4QF3T0Y8V6B" },
    attempts: 0,
    created: -2,
    at: -1,
  },
  {
    key: "queued-later",
    type: "notification.post_subscribers",
    state: "queued",
    payload: { status_id: "01J9Z3X5B1C8R2M7QK0T6Y3H4D" },
    attempts: 0,
    created: -1,
    at: 30,
  },
  {
    key: "retrying",
    type: "federation.deliver",
    state: "retrying",
    payload: { delivery_id: "01J9Z2Q8KX4M6R0N2B5T7V9W1C", inbox: "https://down.example/users/river/inbox" },
    attempts: 3,
    created: -95,
    at: 25,
    ordering: "federation.deliver:down.example",
    errors: [
      "deliver to down.example: dial tcp: connect: connection refused",
      "deliver to down.example: dial tcp: connect: connection refused",
      "deliver to down.example: context deadline exceeded (Client.Timeout exceeded while awaiting headers)",
    ],
  },
  {
    key: "dead-email",
    type: "notification.email",
    state: "dead",
    payload: { notification_id: "01J9YZ8Q1A2B3C4D5E6F7G8H9J", kind: "mention" },
    attempts: 1,
    created: -320,
    at: -318,
    idempotency: "notification.email:01J9YZ8Q1A2B3C4D5E6F7G8H9J",
    errors: ["send mail: the provider rejected the message for good: 550 5.1.1 the mailbox does not exist"],
  },
  {
    key: "dead-deliver",
    type: "federation.deliver",
    state: "dead",
    payload: { delivery_id: "01J9YV1T2R3E4W5Q6A7S8D9F0G", inbox: "https://gone.example/inbox" },
    attempts: 8,
    created: -2880,
    at: -60,
    ordering: "federation.deliver:gone.example",
    errors: Array.from({ length: 8 }, (_, i) =>
      i < 7
        ? "deliver to gone.example: 503 Service Unavailable"
        : "deliver to gone.example: used up its attempts: 503 Service Unavailable",
    ),
  },
  {
    key: "dead-media",
    type: "media.process",
    state: "dead",
    payload: "{not a payload",
    attempts: 1,
    created: -1500,
    at: -1500,
    errors: ["decode media.process: invalid character 'n' looking for beginning of object key string"],
  },
];

/** An error group of the fake as it starts: its key, its group, and its latest occurrence's stack. */
type GroupPlan = { key: string; group: Omit<Group, "id" | "daily_counts">; daily: readonly number[] };

/** The hours, oldest first, that had occurrences: `n` of them an hour apart up to `last` minutes from the start. */
function hours(started: number, counts: readonly number[], last: number, version: string): Group["hourly_counts"] {
  const hour = 60 * minute;
  const end = Math.floor((started + last * minute) / hour) * hour;
  return counts.map((count, i) => ({
    hour: new Date(end - (counts.length - 1 - i) * hour).toISOString(),
    count,
    last_version: version,
  }));
}

const serverStack = `goroutine 214 [running]:
github.com/mycroblog/my/internal/app/status.(*Service).Get(...)
	github.com/mycroblog/my/internal/app/status/get.go:42 +0x32
github.com/mycroblog/my/internal/interface/clientapi.(*Server).getStatus(0xc000412000, {0x1d4e2a0, 0xc0005a8e00}, 0xc0002f6b40)
	github.com/mycroblog/my/internal/interface/clientapi/statuses.go:118 +0x1b4
net/http.HandlerFunc.ServeHTTP(...)
	net/http/server.go:2294 +0x29`;

const webStack = `TypeError: Cannot read properties of undefined (reading 'media_attachments')
    at Media (/assets/thread-Cq4ZkP1x.js:1:20481)
    at renderWithHooks (/assets/react-dom-B2pX9y3a.js:1:90321)`;

function groupPlans(started: number): GroupPlan[] {
  const at = (minutes: number) => new Date(started + minutes * minute).toISOString();
  return [
    {
      key: "panic",
      daily: [0, 0, 0, 0, 2, 9, 14],
      group: {
        fingerprint: "6f1c2a9d4b7e8f0a3c5d1e2b4a6c8d0e",
        state: "open",
        state_changed_at: at(-2 * 24 * 60),
        source: "server",
        kind: "panic",
        where: "client_api GET /api/v1/statuses/{id}",
        type: "runtime.boundsError",
        message: "runtime error: index out of range [3] with length 3",
        count_24h: 14,
        count_7d: 25,
        first_seen_at: at(-2 * 24 * 60),
        last_seen_at: at(-6),
        first_version: "0.42.0",
        last_version: "0.42.1",
        sample: {
          type: "runtime.boundsError",
          message: "runtime error: index out of range [3] with length 3",
          stack: serverStack,
          request_id: "01J9Z3Y7WQ2H6N4K8M0P3R5T7V",
          trace_id: "4bf92f3577b34da6a3ce929d0e0e4736",
          version: "0.42.1",
          role: "all",
          occurred_at: at(-6),
        },
        hourly_counts: hours(started, [2, 1, 4, 1, 3, 5, 1, 6, 2, 2], -6, "0.42.1"),
        events: [],
      },
    },
    {
      key: "web",
      daily: [1, 0, 3, 0, 0, 2, 5],
      group: {
        fingerprint: "a1b2c3d4e5f60718293a4b5c6d7e8f90",
        state: "open",
        state_changed_at: at(-6 * 24 * 60),
        source: "web",
        kind: "error",
        where: "web /_app/statuses/$id",
        type: "TypeError",
        message: "Cannot read properties of undefined (reading 'media_attachments')",
        count_24h: 5,
        count_7d: 11,
        first_seen_at: at(-6 * 24 * 60),
        last_seen_at: at(-40),
        first_version: "0.41.3",
        last_version: "0.42.1",
        sample: {
          type: "TypeError",
          message: "Cannot read properties of undefined (reading 'media_attachments')",
          stack: webStack,
          version: "0.42.1",
          role: "web",
          occurred_at: at(-40),
        },
        hourly_counts: hours(started, [1, 3, 2, 5], -40, "0.42.1"),
        events: [],
      },
    },
    {
      key: "resolved",
      daily: [4, 2, 0, 0, 0, 0, 0],
      group: {
        fingerprint: "0f9e8d7c6b5a49382716051f2e3d4c5b",
        state: "resolved",
        state_changed_at: at(-4 * 24 * 60),
        resolved_version: "0.41.3",
        source: "server",
        kind: "error",
        where: "job federation.deliver",
        type: "*pgconn.PgError",
        message: "ERROR: deadlock detected (SQLSTATE 40P01)",
        count_24h: 0,
        count_7d: 6,
        first_seen_at: at(-9 * 24 * 60),
        last_seen_at: at(-5 * 24 * 60),
        first_version: "0.41.0",
        last_version: "0.41.3",
        sample: {
          type: "*fmt.wrapError > *pgconn.PgError",
          message: "ERROR: deadlock detected (SQLSTATE 40P01)",
          stack: serverStack,
          request_id: "01J9TQ2M4N6P8R0S2T4V6W8X0Y",
          version: "0.41.3",
          role: "worker",
          occurred_at: at(-5 * 24 * 60),
        },
        hourly_counts: hours(started, [2, 1, 3], -5 * 24 * 60, "0.41.3"),
        events: [
          { kind: "resolved", actor: "user", version: "0.41.2", occurred_at: at(-8 * 24 * 60) },
          { kind: "regressed", actor: "system", version: "0.41.3", occurred_at: at(-6 * 24 * 60) },
          { kind: "resolved", actor: "cli", version: "0.41.3", occurred_at: at(-4 * 24 * 60) },
        ],
      },
    },
    {
      key: "muted",
      daily: [30, 28, 31, 29, 33, 30, 27],
      group: {
        fingerprint: "9a8b7c6d5e4f30211203f4e5d6c7b8a9",
        state: "muted",
        state_changed_at: at(-20 * 24 * 60),
        source: "web",
        kind: "error",
        where: "web /_app/",
        type: "ChunkLoadError",
        message: "Loading chunk 812 failed.",
        count_24h: 27,
        count_7d: 208,
        first_seen_at: at(-30 * 24 * 60),
        last_seen_at: at(-12),
        first_version: "0.39.0",
        last_version: "0.42.1",
        hourly_counts: hours(started, [1, 2, 1, 1, 2], -12, "0.42.1"),
        events: [{ kind: "muted", actor: "user", occurred_at: at(-20 * 24 * 60) }],
      },
    },
  ];
}

/** The bounds of the buckets of the UX measurements, as the server's (internal/domain/diagnostics/ux.go). */
const durationBounds = [25, 50, 100, 200, 300, 500, 800, 1000, 1500, 2500, 4000, 6000, 10000];
const percentBounds = [0, 0.5, 1, 2, 5, 10, 20];

/** A measurement of `metric` and `name`, its samples by bucket (one more than the bounds), and its 75th percentile. */
function measurement(
  metric: Schema<"UXMetric">,
  name: string,
  samples: readonly number[],
  scale: number,
): Schema<"UXMeasurementSummary"> {
  const bounds = metric === "call_packet_loss" ? percentBounds : durationBounds;
  const counts = bounds
    .map((_, i) => Math.round((samples[i] ?? 0) * scale))
    .concat(Math.round((samples.at(bounds.length) ?? 0) * scale));
  const total = counts.reduce((a, b) => a + b, 0);
  let sum = 0;
  const at = counts.findIndex((c) => (sum += c) >= total * 0.75);
  const p75 = bounds[at];
  return {
    metric,
    name,
    samples: total,
    ...(p75 === undefined ? {} : { p75 }),
    buckets: counts.map((n, i) => ({ samples: n, ...(i < bounds.length ? { le: bounds[i] } : {}) })),
  };
}

/** The measurements of a window of `hours`, each name's samples growing with it. */
function measurements(hours: number): Schema<"UXMeasurementSummary">[] {
  const scale = hours / 24;
  return [
    measurement("lcp", "/_app/", [0, 0, 2, 10, 40, 120, 210, 160, 90, 40, 12, 4, 1, 0], scale),
    measurement("lcp", "/_app/statuses/$id", [0, 0, 1, 6, 20, 50, 80, 70, 40, 22, 9, 3, 1, 1], scale),
    measurement("lcp", "/_app/notifications", [0, 0, 0, 2, 8, 20, 30, 18, 6, 2, 1, 0, 0, 0], scale),
    measurement("inp", "/_app/", [120, 210, 180, 90, 30, 12, 4, 2, 1, 0, 0, 0, 0, 0], scale),
    measurement("inp", "/_app/statuses/$id", [40, 70, 60, 30, 18, 9, 5, 2, 1, 1, 0, 0, 0, 0], scale),
    measurement("long_frame", "/_app/", [0, 0, 30, 18, 6, 3, 1, 0, 0, 0, 0, 0, 0, 0], scale),
    measurement("transition", "push", [200, 140, 60, 20, 6, 2, 1, 0, 0, 0, 0, 0, 0, 0], scale),
    measurement("transition", "pop", [180, 60, 12, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0], scale),
    measurement("press", "post.reply", [30, 50, 40, 22, 9, 4, 2, 1, 0, 0, 0, 0, 0, 0], scale),
    measurement("press", "post.favourite", [90, 60, 20, 6, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0], scale),
    measurement("call_rtt", "p2p", [0, 4, 12, 9, 3, 1, 0, 0, 0, 0, 0, 0, 0, 0], scale),
    measurement("call_packet_loss", "p2p", [12, 8, 5, 2, 1, 0, 0, 0], scale),
  ];
}

/** The rules of the link card crawler as the fake starts. */
function rulePlans(started: number): Schema<"LinkCardDomainRule">[] {
  const at = (days: number) => new Date(started - days * 24 * 60 * minute).toISOString();
  return [
    {
      domain: "images.example",
      mode: "no_images",
      note: "Their image host asked us not to hotlink.",
      updated_at: at(12),
    },
    { domain: "news.example", mode: "no_images", note: "", updated_at: at(3) },
    {
      domain: "tracker.example",
      mode: "no_fetch",
      note: "Every page of theirs redirects through a tracking service.",
      updated_at: at(40),
    },
  ];
}

/** Whether `domain` is a host name as the server takes it: letters, digits, hyphens and dots, no scheme or port. */
const host = (domain: string) =>
  /^(?=.{1,253}$)[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$/i.test(domain);

export function operationsHandlers(c: OperationsContext): { handlers: Handlers; operations: FakeOperations } {
  const iso = (minutes: number) => new Date(c.started + minutes * minute).toISOString();
  const none = c.options === "none";
  const allowed = (p: Schema<"Permission">) => c.permissions().includes(p);
  const forbidden = () => problem(403, "Forbidden", { detail: "Your account does not have the permission." });
  const keyed = new Map<string, string>();

  // ---- The jobs ---------------------------------------------------------------------------------------------------

  const jobs: Job[] = [];
  const attempts = new Map<string, Attempt[]>();
  for (const [i, plan] of (none ? [] : jobPlans).entries()) {
    const id = idAt(c.started + plan.created * minute, 81_000 + i);
    keyed.set(`job:${plan.key}`, id);
    const errors = plan.errors ?? [];
    jobs.push({
      id,
      type: plan.type,
      state: plan.state,
      payload: typeof plan.payload === "string" ? plan.payload : JSON.stringify(plan.payload),
      attempts: plan.attempts,
      created_at: iso(plan.created),
      ...(plan.idempotency === undefined ? {} : { idempotency_key: plan.idempotency }),
      ...(plan.ordering === undefined ? {} : { ordering_key: plan.ordering }),
      ...(errors.length === 0 ? {} : { last_error: errors.at(-1) ?? "" }),
      ...(plan.state === "dead" ? { failed_at: iso(plan.at) } : { run_at: iso(plan.at) }),
      ...(plan.worker === undefined ? {} : { worker: plan.worker, locked_until: iso(plan.at + 5) }),
    });
    // Each run that failed, a few minutes after the one before, the newest first.
    const span = plan.at - plan.created;
    attempts.set(
      id,
      errors
        .map((error, n): Attempt => {
          const finished = plan.created + (span * (n + 1)) / (errors.length + (plan.state === "dead" ? 0 : 1));
          return {
            attempt: n + 1,
            type: plan.type,
            outcome: plan.state === "dead" && n === errors.length - 1 ? "dead" : "retry",
            error,
            worker: n % 2 === 0 ? "worker-7f3a" : "worker-2c91",
            started_at: iso(finished - 0.02),
            finished_at: iso(finished),
          };
        })
        .toReversed(),
    );
  }
  const pending = (j: Job) => j.state !== "dead";
  const due = (j: Job) => (j.state === "queued" || j.state === "retrying") && Date.parse(j.run_at ?? "") <= Date.now();
  const earliest = (list: readonly Job[]) => list.map((j) => j.run_at ?? "").toSorted()[0];

  const jobHandlers: Handlers = {
    "GET /admin/queue": () => {
      if (!allowed("manage_jobs")) return forbidden();
      const counts = new Map<string, Schema<"AdminJobCount">>();
      for (const j of jobs) {
        const key = `${j.state} ${j.type}`;
        const now = counts.get(key);
        counts.set(key, { state: j.state, type: j.type, count: (now?.count ?? 0) + 1 });
      }
      const waiting = jobs.filter(due);
      const types = [...new Set(waiting.map((j) => j.type))];
      const oldest = earliest(waiting);
      return {
        status: 200,
        body: {
          adapter: "db",
          pending_listing: true,
          counts: [...counts.values()],
          waits: types.map((type) => ({
            type,
            oldest_ready_at: earliest(waiting.filter((j) => j.type === type)) ?? "",
          })),
          ...(oldest === undefined ? {} : { oldest_ready_at: oldest }),
        },
      };
    },
    "GET /admin/jobs": ({ query }) => {
      if (!allowed("manage_jobs")) return forbidden();
      const state = query.get("state");
      const type = query.get("type");
      const kept = jobs
        .filter((j) => (state === null ? pending(j) : j.state === state) && (type === null || j.type === type))
        .toSorted((a, b) =>
          state === "dead"
            ? (b.failed_at ?? "").localeCompare(a.failed_at ?? "")
            : (a.run_at ?? "").localeCompare(b.run_at ?? ""),
        );
      return { status: 200, body: page(kept, query).body };
    },
    "GET /admin/jobs/{id}": ({ params }) => {
      if (!allowed("manage_jobs")) return forbidden();
      const job = jobs.find((j) => j.id === params.id);
      return job === undefined ? notFound() : { status: 200, body: structuredClone(job) };
    },
    "GET /admin/jobs/{id}/attempts": ({ params, query }) => {
      if (!allowed("manage_jobs")) return forbidden();
      const list = attempts.get(params.id) ?? [];
      return { status: 200, body: page(list, query, (a) => `${String(a.attempt)}:${a.finished_at}`).body };
    },
    "POST /admin/jobs/{id}/retry": ({ params }) => {
      if (!allowed("manage_jobs")) return forbidden();
      const job = jobs.find((j) => j.id === params.id && j.state === "dead");
      if (job === undefined) return notFound();
      // Queued to run now with a new count of attempts; the attempts of before stay recorded.
      job.state = "queued";
      job.attempts = 0;
      job.run_at = new Date().toISOString();
      delete job.failed_at;
      return { status: 202 };
    },
  };

  // ---- The error groups -------------------------------------------------------------------------------------------

  const planned = none ? [] : groupPlans(c.started);
  const first = planned[0];
  if (c.options === "crowded" && first !== undefined)
    first.group.hourly_counts = hours(
      c.started,
      Array.from({ length: 30 }, (_, i) => 1 + (i % 6)),
      -6,
      "0.42.1",
    );
  const groups: (Group & { daily: number[] })[] = planned.map((plan, i) => {
    const id = idAt(c.started - (i + 1) * 24 * 60 * minute, 82_000 + i);
    keyed.set(`error:${plan.key}`, id);
    return { id, ...structuredClone(plan.group), daily: [...plan.daily] };
  });
  const listed = ({
    daily,
    sample: _sample,
    hourly_counts: _hours,
    events: _events,
    ...g
  }: Group & { daily: number[] }) => ({
    ...g,
    daily_counts: daily,
  });
  const shown = ({ daily: _daily, ...g }: Group & { daily: number[] }): Group => structuredClone(g);
  const changed = (id: string, change: (g: Group) => void) => {
    if (!allowed("manage_errors")) return forbidden();
    const g = groups.find((x) => x.id === id);
    if (g === undefined) return notFound();
    change(g);
    return { status: 200 as const, body: listed(g) };
  };
  const event = (g: Group, kind: Schema<"ErrorGroupEvent">["kind"], version?: string) => {
    const now = new Date().toISOString();
    g.events.push({ kind, actor: "user", occurred_at: now, ...(version === undefined ? {} : { version }) });
    g.state_changed_at = now;
  };

  const groupHandlers: Handlers = {
    "GET /admin/error_groups": ({ query }) => {
      if (!allowed("manage_errors")) return forbidden();
      const state = query.get("state");
      const source = query.get("source");
      const kept = groups
        .filter((g) => (state === null || g.state === state) && (source === null || g.source === source))
        .toSorted((a, b) => b.last_seen_at.localeCompare(a.last_seen_at))
        .map(listed);
      return { status: 200, body: page(kept, query).body };
    },
    "GET /admin/error_groups/{id}": ({ params }) => {
      if (!allowed("manage_errors")) return forbidden();
      const g = groups.find((x) => x.id === params.id);
      return g === undefined ? notFound() : { status: 200, body: shown(g) };
    },
    "POST /admin/error_groups/{id}/resolve": ({ params }) =>
      changed(params.id, (g) => {
        if (g.state === "resolved") return;
        g.state = "resolved";
        g.resolved_version = g.last_version;
        event(g, "resolved", g.last_version);
      }),
    "POST /admin/error_groups/{id}/mute": ({ params }) =>
      changed(params.id, (g) => {
        if (g.state === "muted") return;
        g.state = "muted";
        delete g.resolved_version;
        event(g, "muted");
      }),
    "POST /admin/error_groups/{id}/unmute": ({ params }) =>
      changed(params.id, (g) => {
        if (g.state !== "muted") return;
        g.state = "open";
        event(g, "unmuted");
      }),
  };

  // ---- The UX measurements ----------------------------------------------------------------------------------------

  const uxHandlers: Handlers = {
    "GET /admin/ux_measurements": ({ query }) => {
      if (!allowed("manage_errors")) return forbidden();
      const asked = Number(query.get("hours") ?? 24);
      if (!Number.isInteger(asked) || asked < 1 || asked > 720)
        return problem(400, "Bad Request", { detail: "hours must be from 1 to 720." });
      const hour = 60 * minute;
      const since = new Date(Math.floor(Date.now() / hour) * hour - (asked - 1) * hour).toISOString();
      // The server takes no sample while the setting is off; the fake keeps none from before.
      const on = c.setting("ux_measurement") === "true" && !none;
      return { status: 200, body: { since, items: on ? measurements(asked) : [] } };
    },
  };

  // ---- The rules of the link card crawler -------------------------------------------------------------------------

  const rules = none ? [] : rulePlans(c.started);
  if (c.options === "crowded")
    for (let i = 0; i < 45; i += 1)
      rules.push({
        domain: `site${String(i + 1).padStart(2, "0")}.example`,
        mode: i % 3 === 0 ? "no_fetch" : "no_images",
        note: "",
        updated_at: new Date(c.started - (i + 1) * 60 * minute).toISOString(),
      });
  const sorted = () => rules.toSorted((a, b) => a.domain.localeCompare(b.domain));
  const ruleHandlers: Handlers = {
    "GET /admin/link_card_domains": () => {
      if (!allowed("manage_settings")) return forbidden();
      return { status: 200, body: { items: structuredClone(sorted()) } };
    },
    "PUT /admin/link_card_domains/{domain}": ({ params, body }) => {
      if (!allowed("manage_settings")) return forbidden();
      const domain = params.domain.trim().toLowerCase();
      if (!host(domain)) return problem(400, "Bad Request", { detail: "The domain is not a host name." });
      if ((body.note ?? "").length > 5000)
        return problem(422, "Unprocessable Content", { detail: "The note is too long.", field: "note" });
      const at = rules.findIndex((r) => r.domain === domain);
      if (at < 0 && rules.length >= 1000)
        return problem(422, "Unprocessable Content", { detail: "There are 1,000 rules already.", field: "rules" });
      const rule = { domain, mode: body.mode, note: body.note ?? "", updated_at: new Date().toISOString() };
      if (at < 0) rules.push(rule);
      else rules.splice(at, 1, rule);
      return { status: 200, body: structuredClone(rule) };
    },
    "DELETE /admin/link_card_domains/{domain}": ({ params }) => {
      if (!allowed("manage_settings")) return forbidden();
      const at = rules.findIndex((r) => r.domain === params.domain);
      if (at >= 0) rules.splice(at, 1);
      return { status: 204 };
    },
  };

  return {
    handlers: { ...jobHandlers, ...groupHandlers, ...uxHandlers, ...ruleHandlers },
    operations: {
      id: (key) => keyed.get(key) ?? "none",
      openErrorGroups: () => groups.filter((g) => g.state === "open").length,
    },
  };
}
