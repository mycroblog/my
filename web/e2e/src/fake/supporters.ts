// SPDX-License-Identifier: AGPL-3.0-only

// The fake's supporters (ADR-0072, ADR-0162, ADR-0070 §13): the plans of the program, the reader's subscription with
// its checkout, changes, cancellation and sync, the provider's portal, the tips with their amounts, checkout and state,
// the supporters list as the instance shows it, and the reader's choice to be on it; each as the server decides it.
// The provider's pages are the server's fake provider's: what the reader would do there is done at once, and the URL
// a start answers is the return to the app with its result (`?checkout=success`, `?checkout=canceled`), so that the
// app goes straight back. As with the server, the payment shows once the server asked the provider again: the sync of
// the return for a subscription, the next read for a tip.

import { day, idAt, minute, type PageQuery } from "./common";
import type { Answer, Handlers, Refusal, Schema } from "./spec";

type Audience = Schema<"InstanceAudience">;
type Account = Schema<"Account">;
type Plan = Schema<"SupporterPlan">;
type Interval = Schema<"SupporterInterval">;
type Subscription = Schema<"SupporterSubscription">;
type Tip = Schema<"SupporterTip">;
type Money = Schema<"Money">;

/**
 * The supporter program of a fake server: on, with two plans (Supporter and Patron, each monthly and yearly, a year at
 * about 17% off), the reader not subscribed, no tips and no list, unless it says otherwise.
 */
export type FakeSupporterOptions = {
  /** The program, on by default: off to take tips alone (`tips`). */
  program?: boolean;
  /**
   * The reader's subscription at the start: none by default; one whose first payment is not made (`pending`); a
   * monthly Supporter (`active`) or a yearly one (`yearly`); one cancelled at the period's end (`canceling`); a monthly
   * Patron whose change to the monthly Supporter waits for the period's end (`scheduled`); one whose payment failed, in
   * its 7 days of grace (`past-due`), or past them (`lapsed`); a monthly Supporter renewing at an earlier, lower price
   * (`kept`, ¥400); or a monthly Founder, a plan of rank 3 the administrators hid, which the public list leaves out
   * (`hidden`).
   */
  subscription?:
    "none" | "pending" | "active" | "kept" | "hidden" | "yearly" | "canceling" | "scheduled" | "past-due" | "lapsed";
  /** Why the server says the reader may not subscribe nor tip: their primary address not confirmed, and so on. */
  ineligible?: Schema<"SupporterIneligibleReason">;
  /** The reader leaves the provider's checkout of a subscription without paying; they pay by default. */
  checkout?: "canceled";
  /** The payment of a change to a higher plan or to the yearly price fails, or has not settled; it succeeds by default. */
  upgrade?: "failed" | "processing";
  /**
   * The tips (¥500, ¥1,000, ¥3,000): off unless given. `open`, the reader left the checkout of a tip unpaid; `paid`, a
   * tip of theirs was paid; `limited`, the reader started more than the server takes (every start answers 429);
   * `checkout: "canceled"`, the reader leaves the next checkout without paying.
   */
  tips?: { open?: boolean; paid?: boolean; limited?: boolean; checkout?: "canceled" };
  /** Who may read the supporters list: nobody by default (`off`), signed-in readers, or everyone. */
  list?: Audience;
  /** The reader chose to be on the list (as a member of a plan). */
  listed?: boolean;
  /** More people of the seed on the Supporter plan, listed, after the others: more than a page of the list. */
  more?: readonly string[];
};

/** What a test does with the fake's supporters. */
export type FakeSupporters = {
  /** The ID of the plan of `key`. */
  readonly planId: (key: "supporter" | "patron") => string;
  /** The ID of the reader's tip of the seed (`open`, `paid`), or of the last one they started (`last`); "none" for none. */
  readonly tipId: (key: "open" | "paid" | "last") => string;
  /** The reader's subscription as the server holds it now; undefined for none. */
  readonly subscription: () => Subscription | undefined;
  /** The members of each plan now, which the income totals count. */
  readonly planCounts: () => Schema<"IncomePlanCount">[];
};

/** What the supporter handlers take from the fake. */
export type SupporterContext = {
  readonly options: FakeSupporterOptions | undefined;
  /** When the seed ran, in ms since the epoch. */
  readonly started: number;
  /** The reader has no session. */
  readonly visitor: boolean;
  /** The service's closure was announced (or it closed): nobody subscribes nor tips. */
  readonly closing: boolean;
  readonly origin: string;
  /** Whether the seed has the person of `username`. */
  readonly known: (username: string) => boolean;
  /** The account of the person of `username`, as the reader sees it; undefined for none of the seed, or one hidden. */
  readonly visibleAccount: (username: string) => Account | undefined;
  readonly viewerAccount: () => Account;
  readonly problem: (status: number, title: string, more?: Partial<Schema<"Problem">>) => Refusal;
  readonly notFound: () => Refusal;
  readonly page: <T>(items: readonly T[], query: PageQuery, key: (item: T) => string) => { body: { items: T[] } };
};

const yen = (amount: number): Money => ({ amount: String(amount), currency: "JPY" });

/** The people of the seed on the plans, besides the reader: their plan, whether they listed, and how long ago. */
const seededMembers: readonly { by: string; plan: "supporter" | "patron"; listedDaysAgo?: number }[] = [
  { by: "hana", plan: "supporter", listedDaysAgo: 10 },
  { by: "sam", plan: "supporter", listedDaysAgo: 3 },
  { by: "mio", plan: "patron", listedDaysAgo: 5 },
  { by: "marco", plan: "patron" },
];

/** The handlers of the supporters, and what a test does with them; the instance's fields of them. */
export function supporterHandlers(c: SupporterContext): {
  handlers: Handlers;
  supporters: FakeSupporters;
  instance: Pick<Schema<"Instance">, "supporters_enabled" | "tips_enabled" | "supporter_list_visibility">;
} {
  const o: FakeSupporterOptions = c.options ?? {};
  const members = [
    ...seededMembers,
    ...(o.more ?? []).map((by, i) => ({ by, plan: "supporter" as const, listedDaysAgo: 20 + i })),
  ];
  const program = c.options !== undefined && o.program !== false;
  const tipsOn = o.tips !== undefined;
  const list: Audience = o.list ?? "off";
  const iso = (ms: number) => new Date(ms).toISOString();
  let ids = 0;
  const newId = () => idAt(Date.now(), 71_000 + (ids += 1));

  const plans: Record<"supporter" | "patron", Plan> = {
    supporter: {
      id: idAt(c.started - 400 * day, 70_001),
      name: "Supporter",
      description: "Keeps the server running. A supporter badge for your profile.",
      rank: 1,
      monthly_price: yen(500),
      yearly_price: yen(5000),
    },
    patron: {
      id: idAt(c.started - 400 * day, 70_002),
      name: "Patron",
      description: "Custom emoji of your own every month.",
      rank: 2,
      monthly_price: yen(1500),
      yearly_price: yen(15000),
    },
  };
  const planList = [plans.supporter, plans.patron];
  /** A plan the administrators hid: it takes no new subscription and the public list leaves it out (ADR-0072 §9). */
  const founder: Plan = {
    id: idAt(c.started - 400 * day, 70_003),
    name: "Founder",
    description: "The first supporters of the server.",
    rank: 3,
    monthly_price: yen(3000),
    yearly_price: yen(30000),
  };
  const planById = (id: string) => planList.find((p) => p.id === id);
  /** A plan a subscription may be of: a public one, or the hidden one. */
  const anyPlan = (id: string) => planById(id) ?? (id === founder.id ? founder : undefined);
  const priceOf = (plan: Plan, interval: Interval) => (interval === "year" ? plan.yearly_price : plan.monthly_price);
  const periodMs = (interval: Interval) => (interval === "year" ? 365 : 30) * day;

  /** The reader's subscription, and whether the provider holds its first payment (the checkout was paid). */
  let own: Subscription | undefined;
  let paidAtProvider = false;
  const subscribed = (plan: Plan, interval: Interval, more: Partial<Subscription> = {}): Subscription => ({
    id: idAt(c.started - 50 * day, 72_001),
    state: "active",
    plan_id: plan.id,
    plan_name: plan.name,
    plan_rank: plan.rank,
    interval,
    price: priceOf(plan, interval),
    period_ends_at: iso(c.started + 12 * day),
    started_at: iso(c.started - 50 * day),
    ...more,
  });
  switch (o.subscription ?? "none") {
    case "none":
      break;
    case "pending":
      own = { ...subscribed(plans.supporter, "month"), state: "pending", started_at: iso(c.started - 10 * minute) };
      delete own.period_ends_at;
      break;
    case "active":
      own = subscribed(plans.supporter, "month");
      break;
    case "kept":
      // Renewing at the price the plan had before the administrators raised it (ADR-0072 §9).
      own = subscribed(plans.supporter, "month", { price: yen(400) });
      break;
    case "hidden":
      own = subscribed(founder, "month");
      break;
    case "yearly":
      own = subscribed(plans.supporter, "year", { period_ends_at: iso(c.started + 200 * day) });
      break;
    case "canceling":
      own = subscribed(plans.supporter, "month", { state: "canceling" });
      break;
    case "scheduled":
      own = subscribed(plans.patron, "month", {
        scheduled_change: {
          plan_id: plans.supporter.id,
          plan_name: plans.supporter.name,
          interval: "month",
          price: plans.supporter.monthly_price,
          effective_at: iso(c.started + 12 * day),
        },
      });
      break;
    case "past-due":
      own = subscribed(plans.supporter, "month", {
        state: "past_due",
        period_ends_at: iso(c.started - 2 * day),
        grace_ends_at: iso(c.started + 5 * day),
      });
      break;
    case "lapsed":
      own = subscribed(plans.supporter, "month", {
        state: "lapsed",
        period_ends_at: iso(c.started - 9 * day),
        end_reason: "lapsed",
      });
      break;
  }
  const member = () => own !== undefined && ["active", "canceling", "past_due"].includes(own.state);
  /** When the reader chose to be listed; undefined while they are not. */
  let listedAt: number | undefined = o.listed === true && member() ? c.started - day : undefined;

  // The tips of the reader, the newest last, with whether the provider holds the payment of an open one.
  const amounts = [yen(500), yen(1000), yen(3000)];
  const tips: { tip: Tip; paidAtProvider: boolean }[] = [];
  const seeded: Partial<Record<"open" | "paid", string>> = {};
  if (o.tips?.paid === true) {
    const at = c.started - day;
    tips.push({
      tip: { id: idAt(at, 73_001), state: "paid", amount: yen(1000), created_at: iso(at), paid_at: iso(at + minute) },
      paidAtProvider: true,
    });
    seeded.paid = idAt(at, 73_001);
  }
  if (o.tips?.open === true) {
    const at = c.started - 10 * minute;
    tips.push({
      tip: { id: idAt(at, 73_002), state: "open", amount: yen(500), created_at: iso(at) },
      paidAtProvider: false,
    });
    seeded.open = idAt(at, 73_002);
  }
  /** When the reader started each tip, for the server's limits (5 an hour). */
  const starts: number[] = [];

  const ineligible = () =>
    c.problem(422, "You cannot support this server", {
      type: "/api/v1/problems/supporter-ineligible",
      detail: "Confirm your email address first.",
    });
  const closingProblem = () => c.problem(409, "The service is closing", { type: "/api/v1/problems/service-closing" });
  const stateProblem = () =>
    c.problem(409, "The subscription cannot do this now", { type: "/api/v1/problems/supporter-subscription-state" });
  const invalid = (field: string) =>
    c.problem(422, "This could not be done.", { detail: "This value is not valid. Check what you entered.", field });
  const validInterval = (interval: unknown): interval is Interval => interval === "month" || interval === "year";
  const ownBody = (): Schema<"OwnSupporterSubscription"> => ({
    ...(own === undefined ? {} : { subscription: own }),
    ...(o.ineligible === undefined ? {} : { ineligible_reason: o.ineligible }),
  });
  const answered = (s: Subscription) => ({ status: 200 as const, body: s });

  /** The supporters list, by plan in the plans' order and the newest first within one, of those the reader sees. */
  const listed = (): Schema<"SupporterListEntry">[] => {
    const all = members.flatMap((m) =>
      m.listedDaysAgo === undefined ? [] : [{ by: m.by, plan: plans[m.plan], at: c.started - m.listedDaysAgo * day }],
    );
    const entries = all.flatMap((m) => {
      const account = c.visibleAccount(m.by);
      return account === undefined ? [] : [{ plan: m.plan, at: m.at, account }];
    });
    if (listedAt !== undefined && own !== undefined && member()) {
      const plan = planById(own.plan_id);
      if (plan !== undefined) entries.push({ plan, at: listedAt, account: c.viewerAccount() });
    }
    return entries
      .toSorted((a, b) => a.plan.rank - b.plan.rank || b.at - a.at)
      .map((e) => ({ plan_id: e.plan.id, account: e.account }));
  };

  const handlers: Handlers = {
    "GET /instance/supporter_plans": () =>
      program ? { status: 200, body: { items: planList.map((p) => ({ ...p })) } } : c.notFound(),

    "GET /supporter/subscription": () => (program ? { status: 200, body: ownBody() } : c.notFound()),

    "POST /supporter/checkout": ({ body }) => {
      if (!program) return c.notFound();
      if (o.ineligible !== undefined) return ineligible();
      if (c.closing) return closingProblem();
      if (!validInterval(body.interval)) return invalid("interval");
      const plan = planById(body.plan_id);
      if (plan === undefined) return c.notFound();
      if (own !== undefined && own.state !== "pending")
        return c.problem(409, "You already support this server", { type: "/api/v1/problems/supporter-subscribed" });
      own = {
        id: own?.id ?? newId(),
        state: "pending",
        plan_id: plan.id,
        plan_name: plan.name,
        plan_rank: plan.rank,
        interval: body.interval,
        price: priceOf(plan, body.interval),
        started_at: iso(Date.now()),
      };
      paidAtProvider = o.checkout !== "canceled";
      const back = `${c.origin}/settings/supporter`;
      return {
        status: 200,
        body: {
          subscription_id: own.id,
          redirect_url: paidAtProvider
            ? `${back}?checkout=success&checkout_id=fake_cs_${own.id}`
            : `${back}?checkout=canceled`,
        },
      };
    },

    "POST /supporter/subscription/sync": () => {
      if (!program) return c.notFound();
      const now = ownBody();
      // The server asked the provider again: a checkout paid there makes the subscription active from the next read.
      if (own?.state === "pending" && paidAtProvider)
        own = { ...own, state: "active", period_ends_at: iso(Date.now() + periodMs(own.interval)) };
      return { status: 200, body: now };
    },

    "POST /supporter/subscription/change": ({ body }): Answer<"POST /supporter/subscription/change"> => {
      if (!program) return c.notFound();
      if (!validInterval(body.interval)) return invalid("interval");
      const plan = planById(body.plan_id);
      if (plan === undefined) return c.notFound();
      if (own?.state !== "active") return stateProblem();
      if (plan.id === own.plan_id && body.interval === own.interval)
        return c.problem(422, "This is your plan already", { type: "/api/v1/problems/supporter-same-plan" });
      const rank = anyPlan(own.plan_id)?.rank ?? own.plan_rank;
      const up = plan.rank !== rank ? plan.rank > rank : body.interval === "year";
      if (!up) {
        own = {
          ...own,
          scheduled_change: {
            plan_id: plan.id,
            plan_name: plan.name,
            interval: body.interval,
            price: priceOf(plan, body.interval),
            effective_at: own.period_ends_at ?? iso(Date.now() + periodMs(own.interval)),
          },
        };
        return answered(own);
      }
      if (o.upgrade === "failed")
        return c.problem(402, "The payment failed", {
          type: "/api/v1/problems/supporter-payment-failed",
          detail: "Update your card, or use another one, and try again.",
        });
      if (o.upgrade === "processing")
        return c.problem(409, "The payment is being processed", {
          type: "/api/v1/problems/supporter-payment-processing",
        });
      const { scheduled_change: _change, ...rest } = own;
      own = {
        ...rest,
        plan_id: plan.id,
        plan_name: plan.name,
        plan_rank: plan.rank,
        interval: body.interval,
        price: priceOf(plan, body.interval),
        ...(body.interval === own.interval ? {} : { period_ends_at: iso(Date.now() + periodMs(body.interval)) }),
      };
      return answered(own);
    },

    "POST /supporter/subscription/cancel": () => {
      if (!program) return c.notFound();
      if (own === undefined || (own.state !== "active" && own.state !== "past_due")) return stateProblem();
      const { scheduled_change: _change, grace_ends_at: _grace, ...rest } = own;
      own = { ...rest, state: "canceling" };
      return answered(own);
    },

    "POST /supporter/subscription/resume": () => {
      if (!program) return c.notFound();
      if (own?.state !== "canceling") return stateProblem();
      own = { ...own, state: "active" };
      return answered(own);
    },

    "POST /supporter/subscription/cancel_scheduled_change": () => {
      if (!program) return c.notFound();
      if (own?.scheduled_change === undefined) return stateProblem();
      const { scheduled_change: _change, ...rest } = own;
      own = rest;
      return answered(own);
    },

    "POST /supporter/portal": () => {
      if (!program) return c.notFound();
      if (own === undefined || own.state === "pending")
        return c.problem(409, "The subscription has ended", { type: "/api/v1/problems/subscription-ended" });
      return { status: 200, body: { url: `${c.origin}/settings/supporter?portal=returned` } };
    },

    "GET /preferences/supporter_listing": () =>
      program && member() ? { status: 200, body: { listed: listedAt !== undefined } } : c.notFound(),

    "PUT /preferences/supporter_listing": ({ body }) => {
      if (!program || !member()) return c.notFound();
      if (body.listed && listedAt === undefined) listedAt = Date.now();
      if (!body.listed) listedAt = undefined;
      return { status: 200, body: { listed: listedAt !== undefined } };
    },

    "GET /instance/supporters": ({ query }) => {
      if (!program || list === "off") return c.notFound();
      if (list === "signed_in" && c.visitor)
        return c.problem(401, "Sign in to see this", { type: "/api/v1/problems/unauthenticated" });
      const planId = query.get("plan_id");
      const entries = listed().filter((e) => planId === null || e.plan_id === planId);
      const { body } = c.page(entries, query, (e) => e.account.id);
      return {
        status: 200,
        body: { ...body, plans: planList.map((p) => ({ id: p.id, name: p.name })) },
        headers: { "X-Robots-Tag": "noindex, noarchive" },
      };
    },

    // A signed-in reader's token grants read:supporters: the answer says why they may not tip.
    "GET /instance/tip_amounts": () =>
      tipsOn
        ? {
            status: 200,
            body: {
              amounts: amounts.map((a) => ({ ...a })),
              ...(c.visitor || o.ineligible === undefined ? {} : { ineligible_reason: o.ineligible }),
            },
          }
        : c.notFound(),

    "POST /supporter/tips": ({ body }) => {
      if (!tipsOn) return c.notFound();
      if (o.ineligible !== undefined) return ineligible();
      if (c.closing) return closingProblem();
      if (!amounts.some((a) => a.amount === body.amount.amount && a.currency === body.amount.currency))
        return c.problem(422, "Choose one of the amounts", {
          type: "/api/v1/problems/supporter-tip-amount",
          detail: "Choose one of the amounts this server takes.",
        });
      const now = Date.now();
      const lastHour = starts.filter((at) => at > now - 60 * minute);
      if (o.tips?.limited === true || lastHour.length >= 5) {
        const retry = o.tips?.limited === true ? 30 * 60 : Math.ceil(((lastHour[0] ?? now) + 60 * minute - now) / 1000);
        return {
          ...c.problem(429, "Too Many Requests", { detail: `Try again in ${String(retry)} seconds.` }),
          headers: { "Retry-After": String(retry) },
        };
      }
      starts.push(now);
      const back = `${c.origin}/settings/supporter/tip`;
      // The open tip first: paid there already, the reader goes to its return; else its checkout ends unpaid.
      const open = tips.find((t) => t.tip.state === "open");
      if (open !== undefined && open.paidAtProvider) {
        open.tip = { ...open.tip, state: "paid", paid_at: iso(now) };
        return {
          status: 200,
          body: { tip_id: open.tip.id, redirect_url: `${back}?tip=${open.tip.id}&checkout=success` },
        };
      }
      if (open !== undefined) open.tip = { ...open.tip, state: "expired" };
      const tip: Tip = { id: newId(), state: "open", amount: { ...body.amount }, created_at: iso(now) };
      const paid = o.tips?.checkout !== "canceled";
      tips.push({ tip, paidAtProvider: paid });
      return {
        status: 200,
        body: { tip_id: tip.id, redirect_url: `${back}?tip=${tip.id}&checkout=${paid ? "success" : "canceled"}` },
      };
    },

    "GET /supporter/tips/{id}": ({ params }) => {
      if (!tipsOn) return c.notFound();
      const held = tips.find((t) => t.tip.id === params.id);
      if (held === undefined) return c.notFound();
      const now = held.tip;
      // A read of an open tip has the server ask the provider again: paid there, it is paid from the next read.
      if (now.state === "open" && held.paidAtProvider) held.tip = { ...now, state: "paid", paid_at: iso(Date.now()) };
      return { status: 200, body: now };
    },
  };

  return {
    handlers,
    supporters: {
      planId: (key) => plans[key].id,
      tipId: (key) => (key === "last" ? tips.at(-1)?.tip.id : seeded[key]) ?? "none",
      subscription: () => own,
      planCounts: () =>
        program
          ? planList.map((p) => ({
              plan_id: p.id,
              name: p.name,
              members:
                members.filter((m) => plans[m.plan].id === p.id && c.known(m.by)).length +
                (member() && own?.plan_id === p.id ? 1 : 0),
            }))
          : [],
    },
    instance: { supporters_enabled: program, tips_enabled: tipsOn, supporter_list_visibility: list },
  };
}
