// SPDX-License-Identifier: AGPL-3.0-only

// The fake's closure of the service as a holder of `manage_service_closure` runs it (FR-808, FR-809, ADR-0088): where
// it stands, from the fake's `closure` (none, announced, closed); announcing it or changing its dates, withdrawing it,
// and closing the server once its closing date has passed, each refused in a state the server refuses it in; what
// closing would end; and, once closed, the progress of the `Delete` of every local account. The instance says the
// closure as it stands now (`instanceOf`), so that the readers' banner and closed screen follow the writes.

import { day, problem } from "./common";
import type { Handlers, Schema } from "./spec";

type Closure = Schema<"AdminServiceClosure">;

/** How the fake's closure starts, past where its `closure` option puts it. */
export type FakeClosureOptions = {
  /** The announced closing date has passed: the server can be closed. */
  due?: boolean;
  /** The announcement gives less than 30 days' notice. */
  shortNotice?: boolean;
  /** Closed, every delivery has ended. */
  delivered?: boolean;
  /** What closing would end holds account moves not ended and accounts under a retention hold. */
  warnings?: boolean;
};

/** What the closure's handlers take from the fake. */
export type ClosureContext = {
  readonly started: number;
  /** Where the closure stands as the fake starts. */
  readonly state: "announced" | "closed" | undefined;
  readonly options: FakeClosureOptions | undefined;
  readonly permissions: () => readonly Schema<"Permission">[];
  /** The fake's domain, which closing it is confirmed by. */
  readonly domain: string;
  /** The operator's message the fake's closure starts with. */
  readonly message: string;
};

/** The fake's closure: its handlers, and the instance as it says the closure now. */
export function closureHandlers(c: ClosureContext): {
  handlers: Handlers;
  instanceOf: (instance: Schema<"Instance">) => Schema<"Instance">;
} {
  const o = c.options ?? {};
  const at = (days: number) => new Date(c.started + days * day).toISOString();
  let closure: Closure =
    c.state === undefined
      ? { state: "none", short_notice: false }
      : c.state === "announced"
        ? {
            state: "announced",
            closes_at: at(o.due === true ? -1 : o.shortNotice === true ? 12 : 60),
            export_until: at(o.due === true ? 29 : o.shortNotice === true ? 42 : 90),
            message: c.message,
            announced_at: at(o.due === true ? -61 : -2),
            changed_at: at(o.due === true ? -61 : -2),
            short_notice: o.shortNotice === true,
          }
        : {
            state: o.delivered === true ? "delivery_completed" : "closed",
            closes_at: at(-5),
            export_until: at(25),
            message: c.message,
            announced_at: at(-65),
            changed_at: at(-5),
            short_notice: false,
            progress: {
              accounts: 1284,
              fanouts_pending: o.delivered === true ? 0 : 37,
              deliveries:
                o.delivered === true
                  ? { pending: 0, succeeded: 61_220, permanent_failure: 412, skipped: 88, dead: 6 }
                  : { pending: 9_851, succeeded: 48_377, permanent_failure: 301, skipped: 64, dead: 0 },
            },
          };
  const check: Schema<"AdminServiceClosureCheck"> = {
    local_accounts: 1284,
    delivery_inboxes: 48,
    estimated_deletes: 1284 * 48,
    live_supporter_subscriptions: 23,
    pending_migrations: o.warnings === true ? 2 : 0,
    retention_holds: o.warnings === true ? 1 : 0,
  };
  const forbidden = () => !c.permissions().includes("manage_service_closure");
  const refused = () => problem(403, "Forbidden", { type: "/api/v1/problems/missing-permission" });
  const wrongState = () =>
    problem(409, "Conflict", {
      type: "/api/v1/problems/service-closure-state",
      detail: "The closure cannot do that in its state.",
    });
  const closed = () => closure.state === "closed" || closure.state === "delivery_completed";
  return {
    instanceOf: (instance) => {
      const { closure: _was, ...rest } = instance;
      if (closure.state === "none" || closure.closes_at === undefined || closure.export_until === undefined)
        return rest;
      return {
        ...rest,
        registrations: { ...instance.registrations, mode: "closed" },
        closure: {
          state: closure.state,
          closes_at: closure.closes_at,
          export_until: closure.export_until,
          ...(closure.message === undefined ? {} : { message: closure.message }),
        },
      };
    },
    handlers: {
      "GET /admin/closure": () => (forbidden() ? refused() : { status: 200, body: closure }),
      "GET /admin/closure/check": () => (forbidden() ? refused() : { status: 200, body: check }),
      "PUT /admin/closure/announcement": ({ body }) => {
        if (forbidden()) return refused();
        if (closed()) return wrongState();
        const closes = Date.parse(body.closes_at);
        const until = body.export_until === undefined ? closes + 30 * day : Date.parse(body.export_until);
        if (Number.isNaN(closes)) return problem(422, "Unprocessable", { field: "closes_at" });
        if (Number.isNaN(until) || until < closes) return problem(422, "Unprocessable", { field: "export_until" });
        if ((body.message ?? "").length > 5000) return problem(422, "Unprocessable", { field: "message" });
        const now = Date.now();
        closure = {
          state: "announced",
          closes_at: new Date(closes).toISOString(),
          export_until: new Date(until).toISOString(),
          ...(body.message === undefined || body.message === "" ? {} : { message: body.message }),
          announced_at: new Date(now).toISOString(),
          changed_at: new Date(now).toISOString(),
          short_notice: closes - now < 30 * day,
        };
        return { status: 200, body: closure };
      },
      "DELETE /admin/closure/announcement": () => {
        if (forbidden()) return refused();
        if (closure.state !== "announced") return wrongState();
        closure = { state: "none", short_notice: false };
        return { status: 200, body: closure };
      },
      "POST /admin/closure/close": ({ body }) => {
        if (forbidden()) return refused();
        if (closure.state !== "announced" || Date.parse(closure.closes_at ?? "") > Date.now()) return wrongState();
        if (body.confirm_domain.trim().toLowerCase() !== c.domain.toLowerCase())
          return problem(422, "Unprocessable", { field: "confirm_domain" });
        closure = {
          ...closure,
          state: "closed",
          changed_at: new Date(Date.now()).toISOString(),
          progress: {
            accounts: check.local_accounts,
            fanouts_pending: check.local_accounts,
            deliveries: { pending: 0, succeeded: 0, permanent_failure: 0, skipped: 0, dead: 0 },
          },
        };
        return { status: 200, body: closure };
      },
    },
  };
}
