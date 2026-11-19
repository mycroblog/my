// SPDX-License-Identifier: AGPL-3.0-only
// The supporters (ADR-0072, ADR-0162, ADR-0070 §13; FR-911, FR-913, FR-914, FR-918, FR-919): the plans where the
// program is on, the reader's subscription and why they may not subscribe, the checkout and the provider's portal (each
// a URL of the payment provider the screen leaves for, in the same tab), the changes of the subscription made on this
// server, the sync on the return from the provider, the tips (the operator's amounts, one paid once through the
// provider, its state on the return), the supporters list as the instance shows it, and the reader's choice to be on it.
// A cancellation, its taking back and the cancel of a scheduled change show from the press on; a change of the plan
// shows once the server answered it, since a higher plan waits for its payment. The writes of the subscription go one
// at a time, and after a refusal it is read again, since what made the server refuse (a payment, the period's end)
// changed it. Nothing of this is offered in the native app's shell (ADR-0072 §14), nor to a reader the server says may
// not: `useSupporterAccess` says who may subscribe and who may tip.
import { inNativeShell } from "@my/platform";
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { inOrder } from "./in-order";
import { instanceQuery } from "./instance";
import type { Money } from "./money";
import { moneyLess, moneyPercent, moneySign, moneyTimes } from "./money";
import { pagedQuery, usePagedListOf, type PagedListOf } from "./paged";
import { useAppData } from "./provider";
import { answered, shownWrite } from "./shown-write";

type Schemas = components["schemas"];

/** A plan that takes subscriptions: its name, description, rank (a higher one includes the lower), and prices. */
export type SupporterPlan = Schemas["SupporterPlan"];
/** How often a subscription is paid: every month or every year. */
export type SupporterInterval = Schemas["SupporterInterval"];
/** The reader's subscription that goes on: its plan, period, price, and what waits for the period's end. */
export type SupporterSubscription = Schemas["SupporterSubscription"];
/** The state of a subscription as the server holds it. */
export type SupporterSubscriptionState = Schemas["SupporterSubscriptionState"];
/** A change to a lower plan or to the monthly price that waits for the end of the period. */
export type SupporterScheduledChange = Schemas["SupporterScheduledChange"];
/** Why the reader may not subscribe nor tip: not a local user, the primary address not confirmed, restricted. */
export type SupporterIneligibleReason = Schemas["SupporterIneligibleReason"];
/** Why a subscription ended. */
export type SupporterEndReason = Schemas["SupporterEndReason"];
/** A tip of the reader: open until it is paid or its checkout ends. */
export type SupporterTip = Schemas["SupporterTip"];
/** The state of a tip. */
export type SupporterTipState = Schemas["SupporterTipState"];
/** A supporter on the list: the plan they are a member of, and their account. */
export type SupporterListEntry = Schemas["SupporterListEntry"];
/** A plan as a heading of the supporters list. */
export type SupporterListPlan = Schemas["SupporterListPlan"];
/** Who may read the supporters list: nobody (`off`), signed-in readers, or everyone. */
export type SupporterListAudience = Schemas["InstanceAudience"];

type OwnSubscription = Schemas["OwnSupporterSubscription"];
type SupporterListPage = Schemas["SupporterListPage"];

/** The intervals of a subscription, the monthly first. */
export const supporterIntervals = ["month", "year"] as const satisfies readonly SupporterInterval[];

/** The states in which the reader is a member of their plan and holds its role. */
const memberStates: ReadonlySet<SupporterSubscriptionState> = new Set(["active", "canceling", "past_due"]);

/** Whether `subscription` makes the reader a member of its plan (`active`, `canceling`, `past_due`). */
export function isSupporterMember(subscription: SupporterSubscription | undefined): boolean {
  return subscription !== undefined && memberStates.has(subscription.state);
}

/** The price of `plan` at `interval`. */
export function planPrice(plan: SupporterPlan, interval: SupporterInterval): Money {
  return interval === "year" ? plan.yearly_price : plan.monthly_price;
}

/**
 * What the yearly price of `plan` saves against twelve monthly ones: the amount and the whole percent, cut toward
 * zero; undefined where it saves nothing or the prices are of two currencies.
 */
export function yearlySaving(plan: SupporterPlan): { readonly saving: Money; readonly percent: number } | undefined {
  const months = moneyTimes(plan.monthly_price, 12);
  const saving = moneyLess(months, plan.yearly_price);
  if (saving === undefined || moneySign(saving) <= 0) return undefined;
  const percent = moneyPercent(saving, months);
  return percent === undefined ? undefined : { saving, percent };
}

/**
 * Where the reader's subscription stands, for the screen that shows it (ADR-0072 §7, §8):
 * - `none`: no subscription goes on (never one, or it ended);
 * - `pending`: started, its first payment not made (a checkout left open, or not settled yet);
 * - `active`: paid, it renews at `renewsAt`; `scheduled` too, with a change that waits for then;
 * - `canceling`: cancelled at the end of its period, `endsAt`, the role held until then;
 * - `past-due`: a payment failed, the role held until `graceEndsAt` (7 days) unless one succeeds;
 * - `lapsed`: the grace ended (or the provider paused it), the role gone until the payment method is fixed.
 */
export type SupporterStanding =
  | { readonly kind: "none" }
  | { readonly kind: "pending" }
  | { readonly kind: "active"; readonly renewsAt: string | undefined }
  | { readonly kind: "scheduled"; readonly renewsAt: string | undefined; readonly change: SupporterScheduledChange }
  | { readonly kind: "canceling"; readonly endsAt: string | undefined }
  | { readonly kind: "past-due"; readonly graceEndsAt: string | undefined }
  | { readonly kind: "lapsed"; readonly reason: SupporterEndReason | undefined };

/** Where `subscription` stands; `none` for none, and for one that ended (expired, revoked, abandoned). */
export function supporterStanding(subscription: SupporterSubscription | undefined): SupporterStanding {
  switch (subscription?.state) {
    case undefined:
    case "expired":
    case "revoked":
    case "abandoned":
      return { kind: "none" };
    case "pending":
      return { kind: "pending" };
    case "active":
      return subscription.scheduled_change === undefined
        ? { kind: "active", renewsAt: subscription.period_ends_at }
        : { kind: "scheduled", renewsAt: subscription.period_ends_at, change: subscription.scheduled_change };
    case "canceling":
      return { kind: "canceling", endsAt: subscription.period_ends_at };
    case "past_due":
      return { kind: "past-due", graceEndsAt: subscription.grace_ends_at };
    case "lapsed":
      return { kind: "lapsed", reason: subscription.end_reason };
  }
}

/**
 * What the reader may do with `subscription` (ADR-0072 §8): change its plan or interval only while `active`, cancel it
 * only while `active` (one whose payment failed stays `past_due` until the payment method is updated or its grace
 * ends: cancelling it changes nothing the reader can see), take the cancellation back while `canceling`, cancel the change that waits for the
 * period's end, and open the provider's portal (payment methods, invoices) once the provider knows it.
 */
export type SupporterActions = {
  readonly change: boolean;
  readonly cancel: boolean;
  readonly resume: boolean;
  readonly cancelScheduledChange: boolean;
  readonly portal: boolean;
};

/** What the reader may do with `subscription`; nothing without one. */
export function supporterActions(subscription: SupporterSubscription | undefined): SupporterActions {
  const state = subscription?.state;
  return {
    change: state === "active",
    cancel: state === "active",
    resume: state === "canceling",
    cancelScheduledChange: subscription?.scheduled_change !== undefined,
    portal: state === "active" || state === "canceling" || state === "past_due" || state === "lapsed",
  };
}

/**
 * When a change of `subscription` to `plan` at `interval` takes effect (ADR-0072 §8): `now`, once the difference,
 * charged pro rata at once, is paid (a higher plan, or the yearly price of the same rank); `period-end` (a lower plan,
 * or the monthly price of the same rank; a lower plan at the yearly price is lower); `same` for its plan and interval.
 * The rank of the plan of `subscription` is the server's (`plan_rank`), also for a hidden plan the public list leaves
 * out.
 */
export function planChangeTiming(
  subscription: SupporterSubscription,
  plan: SupporterPlan,
  interval: SupporterInterval,
): "now" | "period-end" | "same" {
  if (plan.id === subscription.plan_id && interval === subscription.interval) return "same";
  const rank = subscription.plan_rank;
  if (plan.rank !== rank) return plan.rank > rank ? "now" : "period-end";
  return interval === "year" ? "now" : "period-end";
}

// ---- Returning from the provider --------------------------------------------------------------------------------------

/**
 * What the provider returned the reader with, as the address of the screen says it (the server's return URLs): the
 * checkout of a subscription (`/settings/supporter?checkout=success|canceled`), the portal
 * (`/settings/supporter?portal=returned`), or the checkout of a tip (`/settings/supporter/tip?tip=<id>&checkout=…`).
 */
export type SupporterReturn =
  | { readonly kind: "checkout"; readonly paid: boolean }
  | { readonly kind: "portal" }
  | { readonly kind: "tip"; readonly tipId: string; readonly paid: boolean };

/** What the query `search` of a screen's address (with or without its `?`) says the provider returned with; null for none. */
export function supporterReturnOf(search: string): SupporterReturn | null {
  const query = new URLSearchParams(search);
  const checkout = query.get("checkout");
  const tip = query.get("tip");
  const known = checkout === "success" || checkout === "canceled";
  if (tip !== null && tip !== "" && known) return { kind: "tip", tipId: tip, paid: checkout === "success" };
  if (known) return { kind: "checkout", paid: checkout === "success" };
  return query.get("portal") === "returned" ? { kind: "portal" } : null;
}

// ---- Refusals ---------------------------------------------------------------------------------------------------------

/**
 * Why the server refused a write of the supporters, by the problem it answered:
 * - `ineligible`: the reader may not subscribe nor tip (`SupporterRead.ineligibleReason` says why);
 * - `subscribed`: a subscription that is not pending goes on already;
 * - `plan-unavailable`: the plan takes no subscriptions (retired, hidden);
 * - `same-plan`: a change to the plan and interval the subscription has;
 * - `state`: the subscription is in no state for it (a change while not `active`, a resume while not `canceling`);
 * - `payment-failed`: the payment of a change failed, the plan stays (another card, or try again);
 * - `payment-processing`: the payment of a change has not settled, the plan stays until it succeeds;
 * - `ended`: the provider knows no subscription of the reader (the portal);
 * - `unavailable`: the program cannot be used now (no provider, no email);
 * - `closing`: the closure of the service was announced: nobody subscribes nor tips;
 * - `tip-amount`: an amount that is not one of the operator's;
 * - `tip-pending`: another start of a tip of the reader runs at the same time;
 * - `tips-unavailable`: the provider takes no one-time payments now;
 * - `rate-limited`: more tips started than the server takes (`supporterRetryAfterMs` says how long to wait);
 * - `unauthenticated`: no session (the supporters list for signed-in readers);
 * - `not-found`: the program or the tips are off, the plan or the tip unknown, the reader no member (the listing);
 * - `invalid`: a value the server does not take.
 */
export type SupporterRefusal =
  | "ineligible"
  | "subscribed"
  | "plan-unavailable"
  | "same-plan"
  | "state"
  | "payment-failed"
  | "payment-processing"
  | "ended"
  | "unavailable"
  | "closing"
  | "tip-amount"
  | "tip-pending"
  | "tips-unavailable"
  | "rate-limited"
  | "unauthenticated"
  | "not-found"
  | "invalid";

const refusals: Readonly<Record<string, SupporterRefusal>> = {
  "/api/v1/problems/supporter-ineligible": "ineligible",
  "/api/v1/problems/supporter-subscribed": "subscribed",
  "/api/v1/problems/supporter-plan-unavailable": "plan-unavailable",
  "/api/v1/problems/supporter-same-plan": "same-plan",
  "/api/v1/problems/supporter-subscription-state": "state",
  "/api/v1/problems/supporter-payment-failed": "payment-failed",
  "/api/v1/problems/supporter-payment-processing": "payment-processing",
  "/api/v1/problems/subscription-ended": "ended",
  "/api/v1/problems/supporters-unavailable": "unavailable",
  "/api/v1/problems/service-closing": "closing",
  "/api/v1/problems/supporter-tip-amount": "tip-amount",
  "/api/v1/problems/supporter-tip-pending": "tip-pending",
  "/api/v1/problems/tips-unavailable": "tips-unavailable",
};

/** What the server's refusal of a read or a write of the supporters was; null for another failure (the network, a 5xx). */
export function supporterRefusal(error: unknown): SupporterRefusal | null {
  if (!(error instanceof ProblemError)) return null;
  const named = refusals[error.type];
  if (named !== undefined) return named;
  if (error.status === 401) return "unauthenticated";
  if (error.status === 404) return "not-found";
  if (error.status === 429) return "rate-limited";
  return error.status === 422 ? "invalid" : null;
}

/** How long the server asks to wait before the next tip (the `Retry-After` of its 429), in ms; undefined for none. */
export function supporterRetryAfterMs(error: unknown): number | undefined {
  if (!(error instanceof ProblemError) || error.status !== 429) return undefined;
  const value = error.headers.get("Retry-After")?.trim() ?? "";
  if (/^\d+$/.test(value)) return Number(value) * 1000;
  const at = value === "" ? Number.NaN : Date.parse(value);
  return Number.isNaN(at) ? undefined : Math.max(at - Date.now(), 0);
}

// ---- Queries ----------------------------------------------------------------------------------------------------------

/** The plans that take subscriptions, by rank (a 404 while the program is off). */
export function supporterPlansQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/instance/supporter_plans");
}

/** The reader's subscription that goes on, and why they may not subscribe (a 404 while the program is off). */
export function ownSubscriptionQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/supporter/subscription");
}

/** The amounts a tip may be, smallest first, of one currency (a 404 while the server takes no tips). */
export function tipAmountsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/instance/tip_amounts");
}

/** The reader's tip `id`. */
export function supporterTipQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/supporter/tips/{id}", { params: { path: { id } } });
}

/** Whether the reader is on the supporters list (a 404 for a reader who is no member of a plan). */
export function supporterListingQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/preferences/supporter_listing");
}

/** The supporters who chose to be listed (of the plan `planId` alone, with one), a page at a time. */
export function supportersQuery({ api, queries }: AppData, planId?: string) {
  const query = planId === undefined ? {} : { plan_id: planId };
  return pagedQuery<SupporterListEntry, SupporterListPage>(
    queries.queryOptions("get", "/instance/supporters", { params: { query } }).queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/instance/supporters", {
          params: { query: cursor === undefined ? query : { ...query, cursor } },
          signal,
        })
      ).data,
  );
}

/** What a screen reads of one document of the supporters. */
type Read<T> = {
  readonly status: "pending" | "error" | "success";
  /** The failure of the last read (a ProblemError or NetworkError of mykit), or null. */
  readonly error: Error | null;
  /** Reads it again (the reader's Try again). */
  readonly refetch: () => void;
} & T;

const notRead = { status: "pending", error: null } as const;

/** The read of `query` for a screen, whose data `shown` gives; nothing while not enabled. */
function readOfQuery<D, T>(
  query: { data: D | undefined; status: "pending" | "error" | "success"; error: unknown; refetch: () => unknown },
  enabled: boolean,
  shown: (data: D | undefined) => T,
): Read<T> {
  const refetch = () => {
    void query.refetch();
  };
  if (!enabled) return { ...notRead, refetch, ...shown(undefined) };
  return { status: query.status, error: query.error as Error | null, refetch, ...shown(query.data) };
}

/** What a screen reads of the plans: none until the server answered and while not enabled (the program off). */
export type SupporterPlansRead = Read<{ readonly plans: readonly SupporterPlan[] | undefined }>;

/** The plans that take subscriptions; nothing is read, and none given, while `enabled` is false. */
export function useSupporterPlans(enabled = true): SupporterPlansRead {
  const query = useQuery({ ...supporterPlansQuery(useAppData()), enabled });
  return readOfQuery(query, enabled, (data) => ({ plans: data === undefined ? undefined : data.items }));
}

/** What a screen reads of the reader's subscription. */
export type OwnSubscriptionRead = Read<{
  /** Whether the server answered; false until then and while not enabled. */
  readonly read: boolean;
  /** The subscription that goes on; undefined for none, until the server answered and while not enabled. */
  readonly subscription: SupporterSubscription | undefined;
  /** Why the reader may not subscribe nor tip; undefined for a reader who may. */
  readonly ineligibleReason: SupporterIneligibleReason | undefined;
}>;

/**
 * How often a settling read of the subscription or of a tip asks again: every 2 seconds the first 15 times, then every
 * 30 seconds for as long as it settles, so that a screen that says it updates once the payment goes through does.
 */
const settleMs = 2_000;
const settleReads = 15;
const settledLateMs = 30_000;

/** When a read that has been answered `reads` times asks again while it settles. */
const settleAfter = (reads: number) => (reads <= settleReads ? settleMs : settledLateMs);

/**
 * The reader's subscription; nothing is read, and nothing given, while `enabled` is false (the program off, no
 * session). While `settling` (the reader is back from the provider's checkout), a subscription still `pending` is read
 * again every 2 seconds, 15 times, then every 30 seconds, until the server says it is no longer pending.
 */
export function useOwnSubscription(enabled = true, settling = false): OwnSubscriptionRead {
  const query = useQuery({
    ...ownSubscriptionQuery(useAppData()),
    enabled,
    refetchInterval: (q) =>
      settling && q.state.data?.subscription?.state === "pending" ? settleAfter(q.state.dataUpdateCount) : false,
  });
  return readOfQuery(query, enabled, (data) => ({
    read: data !== undefined,
    subscription: data?.subscription,
    ineligibleReason: data?.ineligible_reason,
  }));
}

/** What a screen reads of the tip amounts. */
export type TipAmountsRead = Read<{ readonly amounts: readonly Money[] | undefined }>;

/** The amounts a tip may be; nothing is read, and none given, while `enabled` is false (the tips off). */
export function useTipAmounts(enabled = true): TipAmountsRead {
  const query = useQuery({ ...tipAmountsQuery(useAppData()), enabled });
  return readOfQuery(query, enabled, (data) => ({ amounts: data === undefined ? undefined : data.amounts }));
}

/** What a screen reads of a tip. */
export type SupporterTipRead = Read<{
  /** The tip; undefined until the server answered, once it answered that there is none (404), and while not enabled. */
  readonly tip: SupporterTip | undefined;
  /** Whether the server answered 404: no tip of the reader by that ID. */
  readonly gone: boolean;
}>;

/**
 * The reader's tip `id`, on the return from its checkout: while it is `open` it is read again every 2 seconds, 15
 * times, then every 30 seconds, since each read makes the server ask the provider; nothing is read while `enabled` is
 * false.
 */
export function useSupporterTip(id: string, enabled = true): SupporterTipRead {
  const query = useQuery({
    ...supporterTipQuery(useAppData(), id),
    enabled,
    refetchInterval: (q) => (q.state.data?.state === "open" ? settleAfter(q.state.dataUpdateCount) : false),
  });
  const gone = enabled && query.error instanceof ProblemError && query.error.status === 404;
  return readOfQuery(query, enabled, (data) => ({ tip: gone ? undefined : data, gone }));
}

/** What a screen reads of the reader's choice to be on the supporters list. */
export type SupporterListingRead = Read<{
  /** Whether they are listed; undefined until the server answered, for a reader who is no member, and while not enabled. */
  readonly listed: boolean | undefined;
  /** Whether the server answered 404: the reader is no member of a plan (or the program is off). */
  readonly notMember: boolean;
}>;

/** The reader's choice to be on the supporters list; nothing is read while `enabled` is false (no member). */
export function useSupporterListing(enabled = true): SupporterListingRead {
  const query = useQuery({ ...supporterListingQuery(useAppData()), enabled });
  const notMember = enabled && query.error instanceof ProblemError && query.error.status === 404;
  return readOfQuery(query, enabled, (data) => ({ listed: notMember ? undefined : data?.listed, notMember }));
}

/** The supporters list, with the plans for its headings (`plans`, the lowest first, from its first page). */
export type SupportersList = PagedListOf<SupporterListEntry, SupporterListPage> & {
  /** The plans that are not retired, in their order; undefined until the first page arrived. */
  readonly plans: readonly SupporterListPlan[] | undefined;
};

/**
 * The supporters who chose to be listed, by plan in the plans' order and the newest first within one (of `planId`
 * alone, with one); nothing is read while `enabled` is false (the list `off`, or for signed-in readers without a
 * session, which the server answers 404 and 401).
 */
export function useSupporters(planId?: string, enabled = true): SupportersList {
  const list = usePagedListOf<SupporterListEntry, SupporterListPage>(supportersQuery(useAppData(), planId), enabled);
  return { ...list, plans: list.firstPage?.plans };
}

// ---- Who may ----------------------------------------------------------------------------------------------------------

/**
 * Why the reader is not offered to subscribe or to tip: the program or the tips are off (`off`), the app runs in the
 * native shell (`app`, where nothing of it shows), no session (`signed-out`), a subscription that is not pending goes on
 * already (`subscribed`, subscribing only), or what the server says (`SupporterIneligibleReason`: `email_unverified`
 * for a primary address not confirmed).
 */
export type SupporterBar = "off" | "app" | "signed-out" | "subscribed" | SupporterIneligibleReason;

/** Whether the reader is offered something: `may` undefined until what it is from was read; `bar` why not, when not. */
export type SupporterOffer = { readonly may: boolean | undefined; readonly bar: SupporterBar | undefined };

/** Who may subscribe and who may tip (ADR-0072 §6, §14, ADR-0162 §3, §7), for the screens and the ways to them. */
export type SupporterAccess = {
  /** Whether the program is on (`supporters_enabled`); undefined until the instance was read. */
  readonly program: boolean | undefined;
  /** Whether the server takes tips (`tips_enabled`); undefined until the instance was read. */
  readonly tips: boolean | undefined;
  /** Whether the app runs in the native shell, which shows no plan, no tip, and no way to them. */
  readonly app: boolean;
  /** Subscribing to a plan (a local reader with a confirmed address, without a subscription that goes on). */
  readonly subscribe: SupporterOffer;
  /** Paying a tip (a local reader with a confirmed address; a subscription is no bar). */
  readonly tip: SupporterOffer;
  /** What it is from could not be read: `retry` reads it again. */
  readonly failed: boolean;
  readonly retry: () => void;
};

const offer = (bar: SupporterBar | undefined): SupporterOffer => ({ may: bar === undefined, bar });
const unknown: SupporterOffer = { may: undefined, bar: undefined };

/**
 * Who may subscribe and tip, from the instance and, with a session (`signedIn`), why the server says they may not: for
 * subscribing, where the program is on, the reader's subscription; for tipping, where the server takes tips, the tip
 * amounts, which say it to the reader's token with or without the program. Where the amounts answer 404 (the tips
 * turned off since the instance was read), a tip is offered and the server checks it as it starts.
 */
export function useSupporterAccess(signedIn = true): SupporterAccess {
  const data = useAppData();
  const app = inNativeShell();
  const instance = useQuery(instanceQuery(data));
  const program = instance.data?.supporters_enabled;
  const tips = instance.data?.tips_enabled;
  const asksOwn = signedIn && !app && program === true;
  const asksAmounts = signedIn && !app && tips === true;
  const own = useQuery({ ...ownSubscriptionQuery(data), enabled: asksOwn });
  const amounts = useQuery({ ...tipAmountsQuery(data), enabled: asksAmounts });
  const gone = (error: unknown) => error instanceof ProblemError && error.status === 404;
  const ownUnread = asksOwn && own.data === undefined && !gone(own.error);
  const amountsUnread = asksAmounts && amounts.data === undefined && !gone(amounts.error);
  const retry = () => {
    if (instance.data === undefined) {
      void instance.refetch();
      return;
    }
    if (ownUnread) void own.refetch();
    if (amountsUnread) void amounts.refetch();
  };
  if (instance.data === undefined)
    return { program, tips, app, subscribe: unknown, tip: unknown, failed: instance.isError, retry };
  const before = (on: boolean | undefined): SupporterBar | undefined =>
    on !== true ? "off" : app ? "app" : !signedIn ? "signed-out" : undefined;
  const subscribeBar = before(program);
  const tipBar = before(tips);
  const going = own.data?.subscription;
  const subscribe: SupporterOffer =
    subscribeBar === undefined && ownUnread
      ? unknown
      : offer(
          subscribeBar ??
            own.data?.ineligible_reason ??
            (going !== undefined && going.state !== "pending" ? "subscribed" : undefined),
        );
  const tip: SupporterOffer =
    tipBar === undefined && amountsUnread ? unknown : offer(tipBar ?? amounts.data?.ineligible_reason);
  return {
    program,
    tips,
    app,
    subscribe,
    tip,
    failed: (ownUnread && own.isError) || (amountsUnread && amounts.isError),
    retry,
  };
}

/**
 * What a screen of the supporters waits for: the instance and, with a session, the reader's subscription where the
 * program is on and the tip amounts where the server takes tips.
 */
export async function loadSupporterAccess(data: AppData, signedIn = true): Promise<void> {
  const instance = await data.queryClient.query({ ...instanceQuery(data), staleTime: Infinity }).catch(() => undefined);
  if (!signedIn || inNativeShell() || instance === undefined) return;
  await Promise.all([
    instance.supporters_enabled
      ? data.queryClient.query({ ...ownSubscriptionQuery(data), staleTime: Infinity }).catch(() => undefined)
      : undefined,
    instance.tips_enabled
      ? data.queryClient.query({ ...tipAmountsQuery(data), staleTime: Infinity }).catch(() => undefined)
      : undefined,
  ]);
}

// ---- Writes -----------------------------------------------------------------------------------------------------------

/** The key the writes of the reader's subscription go under, one at a time. */
const subscriptionWrites = "supporter-subscription";

/** The reader's subscription with `change` made to what the cache holds of it. */
function showSubscription(
  data: AppData,
  change: (subscription: SupporterSubscription) => SupporterSubscription | undefined,
): void {
  data.queryClient.setQueryData<OwnSubscription>(ownSubscriptionQuery(data).queryKey, (now) => {
    if (now?.subscription === undefined) return now;
    const next = change(now.subscription);
    return next === undefined ? now : { ...now, subscription: next };
  });
}

/** The reader's subscription and their listing may have changed: both are read again. */
function subscriptionChanged(data: AppData): void {
  void data.queryClient.invalidateQueries({ queryKey: ownSubscriptionQuery(data).queryKey });
  void data.queryClient.invalidateQueries({ queryKey: supporterListingQuery(data).queryKey });
}

/**
 * Runs `write`, a write of the reader's subscription, after the ones made before it, with `show` shown from the press
 * on; once none is on its way, the subscription is what the server last said, or read again after a refusal or a
 * failure with no answer, since the write may have reached the provider.
 */
async function writeSubscription(
  data: AppData,
  show: (subscription: SupporterSubscription) => SupporterSubscription | undefined,
  write: () => Promise<SupporterSubscription>,
): Promise<SupporterSubscription> {
  const key = ownSubscriptionQuery(data).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  return shownWrite<OwnSubscription, SupporterSubscription>(data, subscriptionWrites, {
    keys: [key],
    read: () => data.queryClient.getQueryData<OwnSubscription>(key),
    show: () => {
      showSubscription(data, show);
    },
    write,
    server: (answer) => ({ ...data.queryClient.getQueryData<OwnSubscription>(key), subscription: answer }),
    put: (server) => {
      data.queryClient.setQueryData(key, server);
    },
    refresh: () => {
      subscriptionChanged(data);
    },
    // A change or a cancellation may have reached the provider though its answer did not reach the reader.
    unsure: true,
  });
}

/** Where the provider's page is, for the screen to leave for in the same tab. */
export type SupporterCheckout = {
  /** The subscription it is of, or the tip. */
  readonly id: string;
  /** The provider's page, or, when it was paid already, the return to the app. */
  readonly redirectUrl: string;
};

/**
 * Starts a subscription to `planId` at `interval` and gives the provider's checkout to leave for; once it was paid
 * already, the return to the screen instead. It rejects with the server's refusal (`ineligible`, `subscribed`,
 * `plan-unavailable`, `closing`, `not-found`), and the subscription is read again then.
 */
export async function startSupporterCheckout(
  data: AppData,
  planId: string,
  interval: SupporterInterval,
): Promise<SupporterCheckout> {
  try {
    const checkout = answered(
      await data.api.POST("/supporter/checkout", { body: { plan_id: planId, interval } }),
      "the checkout",
    );
    return { id: checkout.subscription_id, redirectUrl: checkout.redirect_url };
  } catch (error) {
    if (error instanceof ProblemError) subscriptionChanged(data);
    throw error;
  }
}

/** The provider's portal of the reader's payment methods and invoices, to leave for; `ended` when it knows none. */
export async function openSupporterPortal(data: AppData): Promise<string> {
  try {
    return answered(await data.api.POST("/supporter/portal"), "the portal").url;
  } catch (error) {
    if (error instanceof ProblemError) subscriptionChanged(data);
    throw error;
  }
}

/**
 * Changes the reader's subscription to `planId` at `interval`, which shows once the server answered: a higher plan or
 * the yearly price once its difference is paid, a lower one or the monthly price as the change that waits for the
 * period's end. It rejects with the server's refusal (`payment-failed`, `payment-processing`: the plan stays;
 * `same-plan`, `plan-unavailable`, `state`), and the subscription is read again then.
 */
export function changeSupporterSubscription(
  data: AppData,
  planId: string,
  interval: SupporterInterval,
): Promise<SupporterSubscription> {
  return writeSubscription(
    data,
    () => undefined,
    async () =>
      answered(
        await data.api.POST("/supporter/subscription/change", { body: { plan_id: planId, interval } }),
        "the change",
      ),
  );
}

/**
 * Cancels the reader's subscription at the end of its period, which shows from the press on (`canceling`, without the
 * change that waited); it rejects with the server's refusal (`state`), and the subscription is read again then.
 */
export function cancelSupporterSubscription(data: AppData): Promise<SupporterSubscription> {
  return writeSubscription(
    data,
    ({ scheduled_change: _change, ...s }) =>
      s.state === "active" || s.state === "past_due" ? { ...s, state: "canceling" } : undefined,
    async () => answered(await data.api.POST("/supporter/subscription/cancel"), "the cancellation"),
  );
}

/** Takes back the cancellation of the reader's subscription, which shows from the press on (`active`); as `cancel…`. */
export function resumeSupporterSubscription(data: AppData): Promise<SupporterSubscription> {
  return writeSubscription(
    data,
    (s) => (s.state === "canceling" ? { ...s, state: "active" } : undefined),
    async () => answered(await data.api.POST("/supporter/subscription/resume"), "the resumption"),
  );
}

/** Cancels the change that waits for the period's end, which shows from the press on; as `cancel…`. */
export function cancelSupporterScheduledChange(data: AppData): Promise<SupporterSubscription> {
  return writeSubscription(
    data,
    ({ scheduled_change: _change, ...s }) => s,
    async () =>
      answered(await data.api.POST("/supporter/subscription/cancel_scheduled_change"), "the cancel of the change"),
  );
}

/**
 * Tells the server the reader is back from the provider's checkout or portal, which has it ask the provider again; the
 * subscription as it is now shows, and the reader's listing is read again. A screen then reads the subscription while
 * it settles (`useOwnSubscription(true, true)`).
 */
export async function syncSupporterSubscription(data: AppData): Promise<SupporterSubscription | undefined> {
  const key = ownSubscriptionQuery(data).queryKey;
  let own: OwnSubscription;
  try {
    own = await inOrder(data, subscriptionWrites, async () =>
      answered(await data.api.POST("/supporter/subscription/sync"), "the sync"),
    );
  } catch (error) {
    if (error instanceof ProblemError) subscriptionChanged(data);
    throw error;
  }
  await data.queryClient.cancelQueries({ queryKey: key });
  data.queryClient.setQueryData(key, own);
  void data.queryClient.invalidateQueries({ queryKey: supporterListingQuery(data).queryKey });
  return own.subscription;
}

/**
 * Starts a tip of `amount` (one of the operator's) and gives the provider's checkout to leave for; once the open tip
 * was paid already, its return instead. It rejects with the server's refusal (`ineligible`, `tip-amount`,
 * `tip-pending`, `tips-unavailable`, `rate-limited` with `supporterRetryAfterMs`, `closing`, `not-found`).
 */
export async function startSupporterTip(data: AppData, amount: Money): Promise<SupporterCheckout> {
  const tip = answered(
    await data.api.POST("/supporter/tips", { body: { amount: { amount: amount.amount, currency: amount.currency } } }),
    "the tip",
  );
  return { id: tip.tip_id, redirectUrl: tip.redirect_url };
}

/**
 * Lists the reader on the supporters list or takes them off it, which shows from the press on; the choices go one at a
 * time, and once none is on its way it is what the server last said. A reader who is no member (a 404) is read again.
 */
export async function setSupporterListing(data: AppData, listed: boolean): Promise<boolean> {
  const key = supporterListingQuery(data).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  const answer = await shownWrite<{ listed: boolean }, { listed: boolean }>(data, "supporter-listing", {
    keys: [key],
    read: () => data.queryClient.getQueryData<{ listed: boolean }>(key),
    show: () => {
      data.queryClient.setQueryData(key, { listed });
    },
    write: async () =>
      answered(await data.api.PUT("/preferences/supporter_listing", { body: { listed } }), "the listing"),
    server: (answer) => answer,
    put: (server) => {
      data.queryClient.setQueryData(key, server);
    },
    refresh: () => {
      void data.queryClient.invalidateQueries({ queryKey: key });
    },
  });
  void data.queryClient.invalidateQueries({ queryKey: supportersQuery(data).queryKey.slice(0, 2) });
  return answer.listed;
}

/** The writes of the supporters, from a screen, each as the function of the same name says. */
export type SupporterWrites = {
  readonly checkout: (planId: string, interval: SupporterInterval) => Promise<SupporterCheckout>;
  readonly portal: () => Promise<string>;
  readonly change: (planId: string, interval: SupporterInterval) => Promise<SupporterSubscription>;
  readonly cancel: () => Promise<SupporterSubscription>;
  readonly resume: () => Promise<SupporterSubscription>;
  readonly cancelScheduledChange: () => Promise<SupporterSubscription>;
  readonly sync: () => Promise<SupporterSubscription | undefined>;
  readonly tip: (amount: Money) => Promise<SupporterCheckout>;
  readonly setListed: (listed: boolean) => Promise<boolean>;
  /** Reads the plans and the reader's subscription again: after a refusal that says they changed under the reader. */
  readonly reread: () => void;
};

/** The writes of the supporters, from a screen. */
export function useSupporterWrites(): SupporterWrites {
  const data = useAppData();
  return {
    checkout: (planId, interval) => startSupporterCheckout(data, planId, interval),
    portal: () => openSupporterPortal(data),
    change: (planId, interval) => changeSupporterSubscription(data, planId, interval),
    cancel: () => cancelSupporterSubscription(data),
    resume: () => resumeSupporterSubscription(data),
    cancelScheduledChange: () => cancelSupporterScheduledChange(data),
    sync: () => syncSupporterSubscription(data),
    tip: (amount) => startSupporterTip(data, amount),
    setListed: (listed) => setSupporterListing(data, listed),
    reread: () => {
      void data.queryClient.invalidateQueries({ queryKey: supporterPlansQuery(data).queryKey });
      subscriptionChanged(data);
    },
  };
}
