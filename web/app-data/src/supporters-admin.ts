// SPDX-License-Identifier: AGPL-3.0-only
// The supporter program as a holder of `manage_supporters` keeps it (ADR-0072, ADR-0162; FR-910, FR-912, FR-914,
// FR-918, FR-919): the program, on or off, with what it needs and the payment provider of the configuration, which
// no screen changes; the tips and their amounts; the plans, each on a role of its own whose permissions and position
// it sets, created, changed and retired; the subscriptions, a page at a time by plan and state, and ending one at once;
// and the state of the payment provider, with running a reconciliation now. A write puts what the server answered
// into the cache and reads again what it changes: the lists, the instance (what it says of the program and the tips)
// and the public plans.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { instanceQuery } from "./instance";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";

type Schemas = components["schemas"];

/** The supporter program: on or off, what it needs, and the payment provider it was enabled with. */
export type SupporterProgram = Schemas["AdminSupporterProgram"];
/** What the program lacks or should be looked at. */
export type SupporterProgramWarning = Schemas["SupporterProgramWarning"];
/** The tips: on or off, their amounts, and what the provider allows. */
export type SupporterTips = Schemas["AdminSupporterTips"];
/** A plan as a holder of `manage_supporters` reads it, with its role. */
export type AdminSupporterPlan = Schemas["AdminSupporterPlan"];
/** A plan as it is created. */
export type SupporterPlanDraft = Schemas["AdminSupporterPlanCreateRequest"];
/** A change of a plan: of its terms, and of its role's permissions and position; an omitted member keeps its value. */
export type SupporterPlanChange = Schemas["AdminSupporterPlanUpdateRequest"];
/** Which rule a refused plan breaks. */
export type SupporterPlanProblemReason = Schemas["SupporterPlanProblemReason"];
/** Which rule refused amounts of the tips break. */
export type TipAmountsProblemReason = Schemas["TipAmountsProblemReason"];
/** A subscription as a holder of `manage_supporters` reads it. */
export type AdminSubscription = Schemas["AdminSupporterSubscription"];
/** The state of a subscription. */
export type SubscriptionState = Schemas["SupporterSubscriptionState"];
/** The state of the payment provider: its mode, its webhooks, its last reconciliation. */
export type PaymentStatus = Schemas["PaymentStatus"];
/** A role, as a holder of `manage_roles` reads it. */
export type AdminRole = Schemas["AdminRole"];

/** The states of a subscription, those that hold the plan's role first. */
export const subscriptionStates = [
  "active",
  "canceling",
  "past_due",
  "pending",
  "lapsed",
  "expired",
  "revoked",
  "abandoned",
] as const satisfies readonly SubscriptionState[];

/**
 * The permissions a plan's role may hold (FR-912): only those of the `member` group, the benefits a plan can give, in
 * the order of `Permission`.
 */
export const planPermissions = [
  "view_finance",
  "create_custom_emojis",
  "start_voice_rooms",
  "create_invites",
  "keep_posts_longer",
  "create_articles",
  "create_events",
] as const satisfies readonly Schemas["Permission"][];

/** The bounds of a plan's numbers (ADR-0072 §3). */
export const planLimits = {
  name: 50,
  description: 1000,
  discount: 90,
  emoji: 10_000,
  rank: 1_000_000,
} as const;

/** Whether `error` is the server's answer of `status`, and of `type` where given. */
const answered = (error: unknown, status: number, type?: string) =>
  error instanceof ProblemError &&
  error.status === status &&
  (type === undefined || error.type === `/api/v1/problems/${type}`);

/** What a screen reads of one thing: it, once read, and where its read is. */
export type SupporterRead<T> = {
  readonly value: T | undefined;
  readonly status: "pending" | "error" | "success";
  /** The server has no such thing (404): no plan or subscription of that ID, or no payment provider. */
  readonly missing: boolean;
  /** The server refused the reader (403): their roles no longer have the permission. */
  readonly forbidden: boolean;
  readonly refetch: () => void;
};

function readOf<T>(query: {
  data: T | undefined;
  status: "pending" | "error" | "success";
  error: unknown;
  refetch: () => unknown;
}): SupporterRead<T> {
  return {
    value: query.data,
    status: query.status,
    missing: answered(query.error, 404),
    forbidden: answered(query.error, 403),
    refetch: () => {
      void query.refetch();
    },
  };
}

/** Puts `value` under `key` once the reads of it on their way are cancelled. */
async function put<T>(data: AppData, key: readonly unknown[], value: T | undefined): Promise<T> {
  if (value === undefined) throw new Error(`app-data: a write of ${String(key[1])} answered without a body`);
  await data.queryClient.cancelQueries({ queryKey: key });
  data.queryClient.setQueryData(key, value);
  return value;
}

/** Reads again everything of `paths`, and the instance, which says whether the program and the tips are on. */
function changed(data: AppData, ...paths: string[]): void {
  const set = new Set(paths);
  void data.queryClient.invalidateQueries({ predicate: ({ queryKey }) => set.has(String(queryKey[1])) });
  void data.queryClient.invalidateQueries({ queryKey: instanceQuery(data).queryKey });
}

// ---- The program and the tips -----------------------------------------------------------------------------------------

export function supporterProgramQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/supporter_program");
}

export function supporterTipsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/supporter_tips");
}

export function paymentStatusQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/payment_status");
}

/** The supporter program. */
export function useSupporterProgram(): SupporterRead<SupporterProgram> {
  return readOf(useQuery(supporterProgramQuery(useAppData())));
}

/** The tips. */
export function useSupporterTips(): SupporterRead<SupporterTips> {
  return readOf(useQuery(supporterTipsQuery(useAppData())));
}

/** The state of the payment provider; `missing` where the deployment configures none. */
export function usePaymentStatus(): SupporterRead<PaymentStatus> {
  return readOf(useQuery({ ...paymentStatusQuery(useAppData()), retry: false }));
}

/** Reads the program, the tips and the payment provider's state, for a route's loader. */
export async function loadSupporterProgram(data: AppData): Promise<void> {
  const quiet = () => undefined;
  await Promise.all([
    data.queryClient.query(supporterProgramQuery(data)).catch(quiet),
    data.queryClient.query(supporterTipsQuery(data)).catch(quiet),
    data.queryClient.query({ ...paymentStatusQuery(data), retry: false }).catch(quiet),
  ]);
}

/**
 * Why the server refused a change of the program or the tips: the deployment lacks what it needs (`unavailable`: a
 * payment provider, email, or one-time payments for the tips); subscriptions go on (`live-subscriptions`, refusing to
 * turn the program off); the amounts of the tips break a rule (their `reason`); or another failure (`failed`).
 */
export type ProgramRefusal =
  | { readonly kind: "unavailable" }
  | { readonly kind: "live-subscriptions" }
  | { readonly kind: "amounts"; readonly reason: TipAmountsProblemReason | undefined }
  | { readonly kind: "failed" };

/** What the server's refusal of a change of the program or the tips was. */
export function programRefusal(error: unknown): ProgramRefusal {
  if (answered(error, 409, "supporters-unavailable") || answered(error, 409, "tips-unavailable"))
    return { kind: "unavailable" };
  if (answered(error, 409, "live-subscriptions")) return { kind: "live-subscriptions" };
  if (error instanceof ProblemError && error.status === 422)
    return { kind: "amounts", reason: error.problem["reason"] as TipAmountsProblemReason | undefined };
  return { kind: "failed" };
}

/**
 * What a holder of `manage_supporters` does to the program and the tips, and to the payment provider's state; each
 * rejects with the server's problem (`programRefusal`).
 */
export type ProgramActions = {
  /** Turns the program on or off. */
  readonly setProgram: (enabled: boolean) => Promise<SupporterProgram>;
  /** Turns the tips on with `amounts` (1 to 6, in one currency), changes them, or turns them off (undefined). */
  readonly setTips: (amounts: readonly Schemas["Money"][] | undefined) => Promise<SupporterTips>;
  /**
   * Asks for a reconciliation with the payment provider now: `queued`, or `too-soon` where one ran in the last UTC
   * hour (a 429); the state is read again either way.
   */
  readonly reconcile: () => Promise<"queued" | "too-soon">;
};

export function programActions(data: AppData): ProgramActions {
  return {
    setProgram: async (enabled) => {
      const program = await put(
        data,
        supporterProgramQuery(data).queryKey,
        (await data.api.PUT("/admin/supporter_program", { body: { enabled } })).data,
      );
      changed(data, "/admin/supporter_tips", "/instance/supporter_plans");
      return program;
    },
    setTips: async (amounts) => {
      const body = amounts === undefined ? { enabled: false } : { enabled: true, amounts: [...amounts] };
      const tips = await put(
        data,
        supporterTipsQuery(data).queryKey,
        (await data.api.PUT("/admin/supporter_tips", { body })).data,
      );
      changed(data, "/instance/tip_amounts");
      return tips;
    },
    reconcile: async () => {
      const run = await data.api.POST("/admin/payment_reconciliation/runs").then(
        () => "queued" as const,
        (e: unknown) => {
          if (answered(e, 429)) return "too-soon" as const;
          throw e;
        },
      );
      void data.queryClient.invalidateQueries({ queryKey: paymentStatusQuery(data).queryKey });
      return run;
    },
  };
}

/** What a holder of `manage_supporters` does to the program and the tips, for a screen. */
export function useProgramActions(): ProgramActions {
  return programActions(useAppData());
}

// ---- The plans --------------------------------------------------------------------------------------------------------

export function adminPlansQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/supporter_plans");
}

/** Every plan, the retired ones too, by rank. */
export function useAdminPlans(): SupporterRead<readonly AdminSupporterPlan[]> {
  const read = readOf(useQuery(adminPlansQuery(useAppData())));
  return { ...read, value: read.value?.items };
}

/**
 * The plan of `id`, found in the list of the plans, which the API reads whole; `missing` where the list holds none of
 * that ID.
 */
export function useAdminPlan(id: string): SupporterRead<AdminSupporterPlan> {
  const read = useAdminPlans();
  const plan = read.value?.find((p) => p.id === id);
  return { ...read, value: plan, missing: read.missing || (read.value !== undefined && plan === undefined) };
}

/** Reads the plans, for a route's loader. */
export async function loadAdminPlans(data: AppData): Promise<void> {
  await data.queryClient.query(adminPlansQuery(data)).catch(() => undefined);
}

/** The roles, for a holder of `manage_roles`; nothing is read while `enabled` is false. */
export function useAdminRoles(enabled = true): SupporterRead<readonly AdminRole[]> {
  const read = readOf(useQuery({ ...useAppData().queries.queryOptions("get", "/admin/roles"), enabled }));
  return { ...read, value: read.value?.items };
}

/** Which rule a refused plan breaks: its `reason`, the member a 422 names; undefined for another failure. */
export function planRefusal(error: unknown): SupporterPlanProblemReason | "retired" | "unknown-role" | undefined {
  if (answered(error, 409, "supporter-plan-retired")) return "retired";
  if (answered(error, 404)) return "unknown-role";
  if (!(error instanceof ProblemError) || error.status !== 422) return undefined;
  return error.problem["reason"] as SupporterPlanProblemReason | undefined;
}

/** Puts a plan a write answered into the list the cache holds, and reads again what lists the plans. */
async function planWritten(data: AppData, plan: AdminSupporterPlan | undefined): Promise<AdminSupporterPlan> {
  if (plan === undefined) throw new Error("app-data: a write of a plan answered without the plan");
  const key = adminPlansQuery(data).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  data.queryClient.setQueryData<Schemas["AdminSupporterPlanList"]>(key, (now) => {
    if (now === undefined) return now;
    const others = now.items.filter((p) => p.id !== plan.id);
    return { ...now, items: [...others, plan].toSorted((a, b) => a.rank - b.rank) };
  });
  changed(data, "/admin/supporter_plans", "/admin/supporter_subscriptions", "/instance/supporter_plans");
  return plan;
}

/**
 * What a holder of `manage_supporters` does to the plans; each resolves with the plan as the server answered it, and
 * rejects with the server's problem (`planRefusal`).
 */
export type PlanActions = {
  readonly create: (draft: SupporterPlanDraft) => Promise<AdminSupporterPlan>;
  readonly change: (id: string, change: SupporterPlanChange) => Promise<AdminSupporterPlan>;
  /** Retires the plan of `id` for good: its subscriptions end at their period's end, a pending one at once. */
  readonly retire: (id: string) => Promise<AdminSupporterPlan>;
};

export function planActions(data: AppData): PlanActions {
  const path = (id: string) => ({ params: { path: { id } } });
  return {
    create: async (draft) => planWritten(data, (await data.api.POST("/admin/supporter_plans", { body: draft })).data),
    change: async (id, change) =>
      planWritten(data, (await data.api.PATCH("/admin/supporter_plans/{id}", { ...path(id), body: change })).data),
    retire: async (id) => planWritten(data, (await data.api.POST("/admin/supporter_plans/{id}/retire", path(id))).data),
  };
}

/** What a holder of `manage_supporters` does to the plans, for a screen. */
export function usePlanActions(): PlanActions {
  return planActions(useAppData());
}

// ---- The subscriptions ------------------------------------------------------------------------------------------------

/** Which subscriptions a list shows: of one plan and one state, each of every kind where left out. */
export type SubscriptionQuery = {
  readonly plan?: string | undefined;
  readonly state?: SubscriptionState | undefined;
};

/** The subscriptions of `q`, newest first. */
export function adminSubscriptionsQuery({ api, queries }: AppData, q: SubscriptionQuery) {
  const query = {
    ...(q.plan === undefined ? {} : { plan_id: q.plan }),
    ...(q.state === undefined ? {} : { state: q.state }),
  };
  return pagedQuery<AdminSubscription>(
    queries.queryOptions("get", "/admin/supporter_subscriptions", { params: { query } }).queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/admin/supporter_subscriptions", {
          params: { query: cursor === undefined ? query : { ...query, cursor } },
          signal,
        })
      ).data,
  );
}

export function adminSubscriptionQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/supporter_subscriptions/{id}", { params: { path: { id } } });
}

/** The subscriptions of `q`, newest first, a page at a time. */
export function useAdminSubscriptions(q: SubscriptionQuery): PagedList<AdminSubscription> {
  return usePagedList(adminSubscriptionsQuery(useAppData(), q));
}

/** The subscription of `id`. */
export function useAdminSubscription(id: string): SupporterRead<AdminSubscription> {
  return readOf(useQuery(adminSubscriptionQuery(useAppData(), id)));
}

/** Reads the subscriptions of `q`, for a route's loader. */
export async function loadAdminSubscriptions(data: AppData, q: SubscriptionQuery): Promise<void> {
  await data.queryClient.infiniteQuery(adminSubscriptionsQuery(data, q)).catch(() => undefined);
}

/** Reads the subscription of `id`, for a route's loader. */
export async function loadAdminSubscription(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(adminSubscriptionQuery(data, id)).catch(() => undefined);
}

/**
 * Ends the subscription of `id` at once, without a refund: resolves with it as the server answered, or as it is read
 * again where the server says it had ended already (a 409 `subscription-ended`), and reads the lists and the plans'
 * members again.
 */
export async function endSubscription(data: AppData, id: string): Promise<AdminSubscription> {
  const key = adminSubscriptionQuery(data, id).queryKey;
  const ended = await data.api.POST("/admin/supporter_subscriptions/{id}/end", { params: { path: { id } } }).then(
    ({ data: answer }) => answer,
    async (e: unknown) => {
      if (!answered(e, 409, "subscription-ended")) throw e;
      return (await data.api.GET("/admin/supporter_subscriptions/{id}", { params: { path: { id } } })).data;
    },
  );
  const subscription = await put(data, key, ended);
  changed(data, "/admin/supporter_subscriptions", "/admin/supporter_plans");
  return subscription;
}

/** Ends a subscription at once, for a screen. */
export function useEndSubscription(): (id: string) => Promise<AdminSubscription> {
  const data = useAppData();
  return (id) => endSubscription(data, id);
}
