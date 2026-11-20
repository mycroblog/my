// SPDX-License-Identifier: AGPL-3.0-only

// The fake's supporter program as a holder of `manage_supporters` keeps it (ADR-0072, ADR-0162): the program on the
// payment provider of the configuration (Stripe in test mode) with what it needs and what to look at; the tips and
// their amounts; the plans of supporters.ts (Supporter, Patron), the hidden Founder and a retired Early bird, each on
// a role of its own, with the roles the server has; the subscriptions of the seed's people (hana, sam, mio, marco,
// kai and an erased account), a page at a time by plan and state; ending one at once; and the payment provider's
// state with a reconciliation now. Each refusal is the server's problem type and reason.

import { day, idAt, minute, page } from "./common";
import type { Answer, Handlers, Refusal, Schema } from "./spec";

type Plan = Schema<"AdminSupporterPlan">;
type Subscription = Schema<"AdminSupporterSubscription">;
type Role = Schema<"AdminRole">;
type Money = Schema<"Money">;
type State = Schema<"SupporterSubscriptionState">;

/** What the administrators of a fake server keep of its supporter program: as described above unless it says otherwise. */
export type FakeSupportersAdminOptions = {
  /** The program: on by default; off, ready to turn on; or off where the configuration names no payment provider. */
  program?: "on" | "off" | "no-provider";
  /** The commerce disclosure is empty, which the program warns of. */
  noDisclosure?: boolean;
  /** The tips: on with ¥500, ¥1,000 and ¥3,000 by default; off; or on with a provider that takes no one-time payment. */
  tips?: "on" | "off" | "no-one-time";
  /** No plan at all, or a Patron whose prices the provider has not made yet. */
  plans?: "none" | "preparing";
  /** No subscription at all, or 30 more of members of the server, past one page. */
  subscriptions?: "none" | "many";
  /** No webhook delivery is recorded, and the last reconciliation corrected subscriptions. */
  webhook?: "missing";
  /** The last reconciliation failed on the key's permissions. */
  reconciliation?: "failed";
  /** A reconciliation ran in this hour: a new one is a 429. */
  reconcileTooSoon?: boolean;
};

/** What the handlers of the administrators' supporter program take from the fake. */
export type SupportersAdminContext = {
  readonly options: FakeSupportersAdminOptions | undefined;
  /** When the seed ran, in ms since the epoch. */
  readonly started: number;
  /** The account ID of the seed's person of `username`. */
  readonly accountId: (username: string) => string;
  readonly problem: (status: number, title: string, more?: Partial<Schema<"Problem">>) => Refusal;
  readonly notFound: () => Refusal;
};

/** What a story or a test does with the administrators' supporter program of the fake. */
export type FakeSupportersAdmin = {
  /** The ID of the plan of `key` (`supporter`, `patron`, `founder`, `early`; "none" for none). */
  readonly planId: (key: string) => string;
  /** The ID of the subscription of `key` (the subscriber's username, or `erased`; "none" for none). */
  readonly subscriptionId: (key: string) => string;
};

const yen = (amount: number): Money => ({ amount: String(amount), currency: "JPY" });

/** The minor units of `m`, at its currency's digits. */
const minorOf = (m: Money) => Math.round(Number(m.amount) * (m.currency === "JPY" ? 1 : 100));
const moneyOf = (minor: number, currency: string): Money =>
  currency === "JPY" ? yen(minor) : { amount: (minor / 100).toFixed(2), currency };

/** A year of `monthly` less `discount` percent, rounded down to the currency's unit. */
const yearly = (monthly: Money, discount: number): Money =>
  moneyOf(Math.floor((minorOf(monthly) * 12 * (100 - discount)) / 100), monthly.currency);

/** The member permissions, the only ones a plan's role may hold. */
const memberPermissions = new Set<string>([
  "view_finance",
  "create_custom_emojis",
  "start_voice_rooms",
  "create_invites",
  "keep_posts_longer",
  "create_articles",
  "create_events",
]);

/** Whether a subscription goes on, which keeps the program on. */
const goesOn = (s: State) =>
  s === "pending" || s === "active" || s === "canceling" || s === "past_due" || s === "lapsed";

/** Whether a subscription holds its plan's role. */
const holds = (s: State) => s === "active" || s === "canceling" || s === "past_due";

/** The handlers of the administrators' supporter program, and what a story does with them. */
export function supportersAdminHandlers(c: SupportersAdminContext): {
  handlers: Handlers;
  /** The roles its plans are on, where the fake is asked for this administration (`supportersAdmin`); else none. */
  roles: Handlers;
  admin: FakeSupportersAdmin;
} {
  const o: FakeSupportersAdminOptions = c.options ?? {};
  const iso = (ms: number) => new Date(ms).toISOString();
  const created = c.started - 400 * day;
  let ids = 0;
  const newId = () => idAt(Date.now(), 72_000 + (ids += 1));
  const invalid = (type: string, detail: string, reason: string): Refusal => ({
    // The reason, an extension member of the problem, as the server's TipAmountsProblem and SupporterPlanProblem.
    problem: Object.assign(
      { type: `/api/v1/problems/${type}`, title: "Unprocessable Content", status: 422, detail },
      {
        reason,
      },
    ),
  });
  const conflict = (type: string, detail: string) =>
    c.problem(409, "Conflict", { type: `/api/v1/problems/${type}`, detail });

  // ---- The roles and the plans -----------------------------------------------------------------------------------------

  const roles: Role[] = [
    { id: idAt(created, 72_101), key: "admin", position: 1000, permissions: [] },
    {
      id: idAt(created, 72_102),
      key: "moderator",
      position: 100,
      permissions: ["manage_reports", "manage_accounts", "view_audit"],
    },
    { id: idAt(created, 72_103), key: "supporter", position: 10, permissions: ["view_finance"] },
    { id: idAt(created, 72_104), key: "patron", position: 20, permissions: ["view_finance", "create_custom_emojis"] },
    {
      id: idAt(created, 72_105),
      key: "founder",
      position: 30,
      permissions: ["view_finance", "create_custom_emojis", "create_articles"],
    },
    { id: idAt(created, 72_106), key: "early_bird", position: 5, permissions: ["view_finance"] },
    { id: idAt(created, 72_107), key: "listeners", position: 8, permissions: ["start_voice_rooms"] },
  ];
  const roleOf = (key: string) => roles.find((r) => r.key === key);

  const planOf = (
    n: number,
    key: string,
    name: string,
    description: string,
    rank: number,
    monthly: number,
    more: Partial<Plan> = {},
  ): Plan => {
    const role = roleOf(key);
    if (role === undefined) throw new Error(`fake: no role ${key}`);
    return {
      id: idAt(created, 70_000 + n),
      name,
      description,
      rank,
      visibility: "public",
      monthly_price: yen(monthly),
      yearly_price: yearly(yen(monthly), 17),
      annual_discount_percent: 17,
      emoji_limit: rank <= 1 ? 0 : 5 * rank,
      role: { id: role.id, key: role.key, position: role.position, permissions: [...role.permissions] },
      prices_ready: true,
      retired: false,
      members: 0,
      created_at: iso(created),
      revised_at: iso(created),
      ...more,
    };
  };
  const keyed: Record<string, Plan> =
    o.plans === "none"
      ? {}
      : {
          supporter: planOf(
            1,
            "supporter",
            "Supporter",
            "Keeps the server running. A supporter badge for your profile.",
            1,
            500,
          ),
          patron: planOf(2, "patron", "Patron", "Custom emoji of your own every month.", 2, 1500, {
            prices_ready: o.plans !== "preparing",
          }),
          founder: planOf(3, "founder", "Founder", "The first supporters of the server.", 3, 3000, {
            visibility: "hidden",
          }),
          early: planOf(4, "early_bird", "Early bird", "The plan of the server's first month.", 0, 300, {
            retired: true,
            retired_at: iso(c.started - 200 * day),
          }),
        };
  const plans: Plan[] = Object.values(keyed);

  // ---- The subscriptions -------------------------------------------------------------------------------------------------

  const subscriptionOf = (
    n: number,
    username: string | undefined,
    plan: Plan | undefined,
    state: State,
    interval: "month" | "year",
    startedDaysAgo: number,
  ): Subscription | undefined => {
    if (plan === undefined) return undefined;
    const start = c.started - startedDaysAgo * day;
    const length = interval === "year" ? 365 : 30;
    const periodEnd = start + Math.ceil(startedDaysAgo / length + 0.0001) * length * day;
    return {
      id: idAt(start, 73_000 + n),
      state,
      plan_id: plan.id,
      plan_name: plan.name,
      interval,
      price: interval === "year" ? plan.yearly_price : plan.monthly_price,
      ...(state === "pending" || state === "abandoned" ? {} : { period_ends_at: iso(periodEnd) }),
      ...(state === "past_due" ? { grace_ends_at: iso(c.started + 5 * day) } : {}),
      ...(username === undefined ? {} : { account_id: c.accountId(username), username }),
      started_at: iso(start),
    };
  };
  const byKey = new Map<string, Subscription>();
  const seed: [string, string | undefined, string, State, "month" | "year", number][] =
    o.subscriptions === "none"
      ? []
      : [
          ["hana", "hana", "supporter", "active", "month", 95],
          ["sam", "sam", "supporter", "active", "year", 40],
          ["mio", "mio", "patron", "canceling", "month", 70],
          ["marco", "marco", "patron", "past_due", "month", 62],
          ["kai", "kai", "founder", "pending", "month", 0.1],
          ["erased", undefined, "early", "expired", "month", 230],
        ];
  seed.forEach(([key, username, plan, state, interval, days], i) => {
    const s = subscriptionOf(i + 1, username, keyed[plan], state, interval, days);
    if (s !== undefined) byKey.set(key, s);
  });
  if (o.subscriptions === "many")
    for (let i = 1; i <= 30; i++) {
      const key = `member${String(i)}`;
      const s = subscriptionOf(100 + i, key, keyed["supporter"], i % 7 === 0 ? "expired" : "active", "month", 120 + i);
      if (s !== undefined) byKey.set(key, { ...s, account_id: idAt(created, 74_000 + i) });
    }
  const subscriptions = (): Subscription[] =>
    [...byKey.values()].toSorted((a, b) => b.started_at.localeCompare(a.started_at));
  const counted = (plan: Plan): Plan => ({
    ...plan,
    members: subscriptions().filter((s) => s.plan_id === plan.id && holds(s.state)).length,
  });

  // ---- The program, the tips, the provider ---------------------------------------------------------------------------------

  const configured = o.program !== "no-provider";
  let enabled = o.program === undefined || o.program === "on";
  let changedAt = iso(c.started - 300 * day);
  const oneTime = o.tips !== "no-one-time";
  let tipAmounts: Money[] = o.tips === "off" ? [] : [yen(500), yen(1000), yen(3000)];
  let tipsChanged: string | undefined = o.tips === "off" ? undefined : iso(c.started - 60 * day);
  const limits = [
    { minimum: yen(100), maximum: yen(100_000) },
    { minimum: { amount: "1.00", currency: "USD" }, maximum: { amount: "1000.00", currency: "USD" } },
  ];

  const program = (): Schema<"AdminSupporterProgram"> => {
    const warnings: Schema<"SupporterProgramWarning">[] = [
      ...(configured ? [] : (["no_provider"] as const)),
      ...(o.noDisclosure === true ? (["no_commerce_disclosure"] as const) : []),
    ];
    return {
      enabled,
      ...(enabled ? { provider: "stripe" } : {}),
      ...(configured ? { configured_provider: "stripe" } : {}),
      sends_email: true,
      commerce_disclosure: o.noDisclosure !== true,
      live_subscriptions: subscriptions().filter((s) => goesOn(s.state)).length,
      can_enable: !enabled && configured,
      changed_at: changedAt,
      warnings,
    };
  };
  const tips = (): Schema<"AdminSupporterTips"> => {
    const on = tipAmounts.length > 0;
    return {
      enabled: on,
      accepting: on && enabled && oneTime,
      one_time_payments: oneTime,
      amounts: tipAmounts,
      limits,
      server_limit_reached: false,
      ...(tipsChanged === undefined ? {} : { changed_at: tipsChanged }),
    };
  };
  let reconciledAt = c.started - 2 * 60 * minute;
  const paymentStatus = (): Schema<"PaymentStatus"> => ({
    provider: "stripe",
    mode: "test",
    time_zone: "UTC",
    ...(o.webhook === "missing" ? {} : { last_webhook_at: iso(c.started - 25 * minute) }),
    last_reconciliation:
      o.reconciliation === "failed"
        ? {
            outcome: "failed",
            error_kind: "permission",
            requests: 1,
            checked_subscriptions: 0,
            corrected_subscriptions: 0,
            occurred_at: iso(reconciledAt),
          }
        : {
            outcome: "succeeded",
            error_kind: "none",
            requests: 3,
            checked_subscriptions: 4,
            corrected_subscriptions: o.webhook === "missing" ? 2 : 0,
            occurred_at: iso(reconciledAt),
          },
    // A reconciliation that corrected subscriptions is a sign only where it ran.
    warnings:
      o.webhook === "missing"
        ? ["no_webhook", ...(o.reconciliation === "failed" ? [] : ["reconciliation_corrected" as const])]
        : [],
  });

  // ---- The checks of a plan --------------------------------------------------------------------------------------------

  const planProblem = (reason: Schema<"SupporterPlanProblemReason">, detail: string) =>
    invalid("invalid-supporter-plan", detail, reason);
  /** What breaks a rule of the terms `p`, for `self` (none for a new plan), or undefined. */
  const termsFault = (
    p: Partial<Schema<"AdminSupporterPlanCreateRequest">>,
    self: Plan | undefined,
  ): Refusal | undefined => {
    if (p.name !== undefined && (p.name.trim() === "" || p.name.length > 50 || p.name.includes("\n")))
      return planProblem("name", "Write a name of 1 to 50 characters on one line.");
    if (p.description !== undefined && p.description.length > 1000)
      return planProblem("description", "Write a description of up to 1,000 characters.");
    if (p.monthly_price !== undefined) {
      if (p.monthly_price.currency !== "JPY" && p.monthly_price.currency !== "USD")
        return planProblem("currency", "Choose yen or US dollars.");
      const digits = p.monthly_price.currency === "JPY" ? /^\d+$/ : /^\d+(\.\d{1,2})?$/;
      if (!digits.test(p.monthly_price.amount) || Number(p.monthly_price.amount) <= 0)
        return planProblem("monthly_price", "Write a price above zero with the currency's digits.");
    }
    if (p.annual_discount_percent !== undefined && (p.annual_discount_percent < 0 || p.annual_discount_percent > 90))
      return planProblem("annual_discount_percent", "Write a discount from 0 to 90.");
    if (p.emoji_limit !== undefined && (p.emoji_limit < 0 || p.emoji_limit > 10_000))
      return planProblem("emoji_limit", "Write a number from 0 to 10,000.");
    if (p.rank !== undefined) {
      if (p.rank < 0 || p.rank > 1_000_000) return planProblem("rank", "Write an order from 0 to 1,000,000.");
      if (plans.some((q) => !q.retired && q !== self && q.rank === p.rank))
        return planProblem("rank", "Another plan has this order. Choose another.");
    }
    return undefined;
  };

  const handlers: Handlers = {
    "GET /admin/supporter_program": (): Answer<"GET /admin/supporter_program"> => ({ status: 200, body: program() }),
    "PUT /admin/supporter_program": ({ body }): Answer<"PUT /admin/supporter_program"> => {
      if (body.enabled !== enabled) {
        if (body.enabled && !configured)
          return conflict("supporters-unavailable", "The server has no payment provider in its configuration.");
        if (!body.enabled && program().live_subscriptions > 0)
          return conflict("live-subscriptions", "Subscriptions go on. Retire the plans first.");
        enabled = body.enabled;
        changedAt = iso(Date.now());
      }
      return { status: 200, body: program() };
    },

    "GET /admin/supporter_tips": (): Answer<"GET /admin/supporter_tips"> => ({ status: 200, body: tips() }),
    "PUT /admin/supporter_tips": ({ body }): Answer<"PUT /admin/supporter_tips"> => {
      if (!body.enabled) {
        tipAmounts = [];
        tipsChanged = iso(Date.now());
        return { status: 200, body: tips() };
      }
      if (!enabled || !oneTime)
        return conflict(
          "tips-unavailable",
          "Tips need the supporter program on a provider that takes one-time payments.",
        );
      const amounts = body.amounts ?? [];
      const distinct = new Set(amounts.map((m) => `${m.currency}:${String(minorOf(m))}`));
      if (amounts.length < 1 || amounts.length > 6 || distinct.size !== amounts.length)
        return invalid("invalid-tip-amounts", "Choose 1 to 6 different amounts.", "count");
      if (new Set(amounts.map((m) => m.currency)).size !== 1)
        return invalid("invalid-tip-amounts", "Write every amount in one currency.", "currency");
      for (const m of amounts) {
        const limit = limits.find((l) => l.minimum.currency === m.currency);
        if (limit === undefined) return invalid("invalid-tip-amounts", "Choose yen or US dollars.", "currency");
        if (minorOf(m) < minorOf(limit.minimum) || minorOf(m) > minorOf(limit.maximum))
          return invalid("invalid-tip-amounts", "An amount is out of range.", "range");
      }
      tipAmounts = amounts.toSorted((a, b) => minorOf(a) - minorOf(b));
      tipsChanged = iso(Date.now());
      return { status: 200, body: tips() };
    },

    "GET /admin/supporter_plans": (): Answer<"GET /admin/supporter_plans"> => ({
      status: 200,
      // A fake not asked for this administration (`supportersAdmin`) has no plan, so that what reads the plans
      // elsewhere (the administration of roles, whose rights name the plans' roles) meets none of these.
      body: { items: c.options === undefined ? [] : plans.toSorted((a, b) => a.rank - b.rank).map(counted) },
    }),
    "POST /admin/supporter_plans": ({ body }): Answer<"POST /admin/supporter_plans"> => {
      if (!configured)
        return conflict("supporters-unavailable", "The server has no payment provider in its configuration.");
      const role = roleOf(body.role);
      if (role === undefined) return c.notFound();
      if (role.key === "admin" || role.key === "moderator")
        return planProblem("role_builtin", "A built-in role cannot be a plan's.");
      if (plans.some((p) => p.role.key === role.key)) return planProblem("role_taken", "Another plan has this role.");
      if (role.position >= 100) return planProblem("role_position", "The role must stand below the moderators.");
      if (role.permissions.some((p) => !memberPermissions.has(p)))
        return planProblem("role_permissions", "The role may hold member permissions alone.");
      const fault = termsFault(body, undefined);
      if (fault !== undefined) return fault;
      const now = iso(Date.now());
      const plan: Plan = {
        id: newId(),
        name: body.name.trim(),
        description: body.description ?? "",
        rank: body.rank,
        visibility: body.visibility,
        monthly_price: body.monthly_price,
        yearly_price: yearly(body.monthly_price, body.annual_discount_percent),
        annual_discount_percent: body.annual_discount_percent,
        emoji_limit: body.emoji_limit,
        role: { id: role.id, key: role.key, position: role.position, permissions: [...role.permissions] },
        prices_ready: true,
        retired: false,
        members: 0,
        created_at: now,
        revised_at: now,
      };
      plans.push(plan);
      return { status: 201, body: plan };
    },
    "PATCH /admin/supporter_plans/{id}": ({ params, body }): Answer<"PATCH /admin/supporter_plans/{id}"> => {
      const plan = plans.find((p) => p.id === params.id);
      if (plan === undefined) return c.notFound();
      if (plan.retired) return conflict("supporter-plan-retired", "The plan is retired.");
      const fault = termsFault(body, plan);
      if (fault !== undefined) return fault;
      if (body.role_position !== undefined && (body.role_position < 0 || body.role_position >= 100))
        return planProblem("role_position", "The role must stand below the moderators.");
      if (body.role_permissions?.some((p) => !memberPermissions.has(p)) === true)
        return planProblem("role_permissions", "The role may hold member permissions alone.");
      const discount = body.annual_discount_percent ?? plan.annual_discount_percent;
      const monthly = body.monthly_price ?? plan.monthly_price;
      Object.assign(plan, {
        ...(body.name === undefined ? {} : { name: body.name.trim() }),
        ...(body.description === undefined ? {} : { description: body.description }),
        ...(body.rank === undefined ? {} : { rank: body.rank }),
        ...(body.visibility === undefined ? {} : { visibility: body.visibility }),
        ...(body.emoji_limit === undefined ? {} : { emoji_limit: body.emoji_limit }),
        monthly_price: monthly,
        annual_discount_percent: discount,
        yearly_price: yearly(monthly, discount),
        role: {
          ...plan.role,
          ...(body.role_position === undefined ? {} : { position: body.role_position }),
          ...(body.role_permissions === undefined ? {} : { permissions: [...body.role_permissions] }),
        },
        revised_at: iso(Date.now()),
      });
      return { status: 200, body: counted(plan) };
    },
    "POST /admin/supporter_plans/{id}/retire": ({ params }): Answer<"POST /admin/supporter_plans/{id}/retire"> => {
      const plan = plans.find((p) => p.id === params.id);
      if (plan === undefined) return c.notFound();
      if (plan.retired) return conflict("supporter-plan-retired", "The plan is retired.");
      plan.retired = true;
      plan.retired_at = iso(Date.now());
      for (const [key, s] of byKey)
        if (s.plan_id === plan.id) {
          if (s.state === "pending") byKey.set(key, { ...s, state: "abandoned" });
          else if (s.state === "active" || s.state === "past_due") byKey.set(key, { ...s, state: "canceling" });
          else if (s.state === "lapsed") byKey.set(key, { ...s, state: "expired" });
        }
      return { status: 200, body: counted(plan) };
    },

    "GET /admin/supporter_subscriptions": ({ query }): Answer<"GET /admin/supporter_subscriptions"> => {
      const plan = query.get("plan_id");
      const state = query.get("state");
      const shown = subscriptions().filter(
        (s) => (plan === null || s.plan_id === plan) && (state === null || s.state === state),
      );
      return { status: 200, body: page(shown, query).body };
    },
    "GET /admin/supporter_subscriptions/{id}": ({ params }): Answer<"GET /admin/supporter_subscriptions/{id}"> => {
      const s = subscriptions().find((x) => x.id === params.id);
      return s === undefined ? c.notFound() : { status: 200, body: s };
    },
    "POST /admin/supporter_subscriptions/{id}/end": ({
      params,
    }): Answer<"POST /admin/supporter_subscriptions/{id}/end"> => {
      const entry = [...byKey].find(([, s]) => s.id === params.id);
      if (entry === undefined) return c.notFound();
      const [key, s] = entry;
      if (!goesOn(s.state)) return conflict("subscription-ended", "The subscription has ended.");
      const { grace_ends_at: _grace, ...rest } = s;
      const ended: Subscription = { ...rest, state: s.state === "pending" ? "abandoned" : "expired" };
      byKey.set(key, ended);
      return { status: 200, body: ended };
    },

    "GET /admin/payment_status": (): Answer<"GET /admin/payment_status"> =>
      configured ? { status: 200, body: paymentStatus() } : c.notFound(),
    "POST /admin/payment_reconciliation/runs": (): Answer<"POST /admin/payment_reconciliation/runs"> => {
      if (!configured) return c.notFound();
      if (o.reconcileTooSoon === true)
        return c.problem(429, "Too Many Requests", { detail: "A reconciliation ran in this hour." });
      reconciledAt = Date.now();
      return { status: 202 };
    },
  };

  return {
    handlers,
    roles:
      c.options === undefined
        ? {}
        : {
            "GET /admin/roles": (): Answer<"GET /admin/roles"> => ({
              status: 200,
              body: { items: roles.toSorted((a, b) => b.position - a.position) },
            }),
          },
    admin: {
      planId: (key) => keyed[key]?.id ?? "none",
      subscriptionId: (key) => byKey.get(key)?.id ?? "none",
    },
  };
}
