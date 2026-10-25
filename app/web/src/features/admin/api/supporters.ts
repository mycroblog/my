// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the supporters screen (ADR-0072 §2, §3, §9, §10, manage_supporters, web-client/README.md S1):
// the supporter program (GET / PUT /admin/supporter_program), every plan, retired ones too (GET /admin/supporter_plans),
// creating, changing and retiring one, and the subscriptions (GET /admin/supporter_subscriptions, newest first, as an
// infinite query narrowed to a plan and a state) with cancelling one at once, and the state of the payment provider
// (GET /admin/payment_status, ADR-0074 §4) with reconciling with it now (POST /admin/payment_reconciles, once an hour). A
// plan that breaks a rule is a 422 `invalid-supporter-plan` problem whose `reason` says which. Every change is recorded
// in the audit log.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { pageQuery, refreshAuditLog, rolesQueryOptions } from "../../moderation";

type Schemas = components["schemas"];

export type SupporterProgram = Schemas["AdminSupporterProgram"];
export type SupporterProgramWarning = Schemas["SupporterProgramWarning"];
export type SupporterPlan = Schemas["AdminSupporterPlan"];
export type NewSupporterPlan = Schemas["AdminSupporterPlanCreateRequest"];
export type SupporterPlanChange = Schemas["AdminSupporterPlanUpdateRequest"];
export type SupporterPlanVisibility = Schemas["SupporterPlanVisibility"];
export type SupporterPlanProblemReason = Schemas["SupporterPlanProblemReason"];
export type SupporterSubscription = Schemas["AdminSupporterSubscription"];
export type SupporterSubscriptionState = Schemas["SupporterSubscriptionState"];
export type PaymentStatus = Schemas["PaymentStatus"];
export type PaymentStatusWarning = PaymentStatus["warnings"][number];
export type PaymentReconcileRun = Schemas["PaymentReconcileRun"];

/** The visibilities of a plan, in the order of the form. */
export const supporterPlanVisibilities = ["public", "hidden"] as const satisfies readonly SupporterPlanVisibility[];

/** The states of a subscription, in the order of the filter (SupporterSubscriptionState). */
export const supporterSubscriptionStates = [
  "pending",
  "active",
  "canceling",
  "past_due",
  "lapsed",
  "expired",
  "revoked",
  "abandoned",
] as const satisfies readonly SupporterSubscriptionState[];

/** The bounds of a plan (AdminSupporterPlanCreateRequest). */
export const supporterPlanNameLimit = 50;
export const supporterPlanDescriptionLimit = 1000;
export const maxAnnualDiscount = 90;
export const maxEmojiLimit = 10_000;
export const maxPlanRank = 1_000_000;

/** The problem type of a plan that breaks a rule. */
const invalidPlanProblem = "/api/v1/problems/invalid-supporter-plan";

const planProblemReasons: readonly string[] = [
  "name",
  "description",
  "monthly_price",
  "currency",
  "annual_discount_percent",
  "emoji_limit",
  "rank",
  "visibility",
  "role_builtin",
  "role_taken",
  "role_held",
  "role_permissions",
  "role_position",
  "order_rank",
  "order_position",
  "order_permissions",
  "order_emoji_limit",
] satisfies readonly SupporterPlanProblemReason[];

/** The reason of an `invalid-supporter-plan` problem, or undefined for another error or a problem without one. */
export function supporterPlanProblemReason(error: unknown): SupporterPlanProblemReason | undefined {
  if (!(error instanceof ApiError) || error.type !== invalidPlanProblem) return undefined;
  const reason = error.document["reason"];
  return typeof reason === "string" && planProblemReasons.includes(reason) ? (reason as SupporterPlanProblemReason) : undefined;
}

/** The supporter program. */
export function supporterProgramQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/supporter_program");
}

/** Every plan, retired ones too, by rank. */
export function supporterPlansQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/supporter_plans");
}

/** The state of the payment provider of the configuration; a 404 without one, so it is read only when the program names one. */
export function paymentStatusQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/payment_status");
}

/** Which subscriptions the list shows: those of one plan and in one state; undefined keeps every one. */
export type SupporterSubscriptionFilter = { planId: string | undefined; state: SupporterSubscriptionState | undefined };

/** The subscriptions, newest first (GET /admin/supporter_subscriptions). */
export function supporterSubscriptionsQueryOptions(api: Api, filter: SupporterSubscriptionFilter) {
  const narrow = { ...(filter.planId === undefined ? {} : { plan_id: filter.planId }), ...(filter.state === undefined ? {} : { state: filter.state }) };
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/supporter_subscriptions", narrow, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/supporter_subscriptions", { params: { query: { ...pageQuery(pageParam), ...narrow } }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Enables or disables the program (PUT /admin/supporter_program), as the options of useMutation. */
export function setSupporterProgramMutation(api: Api) {
  return {
    mutationKey: ["put", "/admin/supporter_program"] as const,
    mutationFn: async (enabled: boolean): Promise<SupporterProgram> => {
      const { data, error } = await api.fetch.PUT("/admin/supporter_program", { body: { enabled } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Creates a plan (POST /admin/supporter_plans), as the options of useMutation. */
export function createSupporterPlanMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/supporter_plans"] as const,
    mutationFn: async (body: NewSupporterPlan): Promise<SupporterPlan> => {
      const { data, error } = await api.fetch.POST("/admin/supporter_plans", { body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Changes a plan (PATCH /admin/supporter_plans/{id}), as the options of useMutation. */
export function updateSupporterPlanMutation(api: Api) {
  return {
    mutationKey: ["patch", "/admin/supporter_plans/{id}"] as const,
    mutationFn: async ({ id, change }: { id: string; change: SupporterPlanChange }): Promise<SupporterPlan> => {
      const { data, error } = await api.fetch.PATCH("/admin/supporter_plans/{id}", { params: { path: { id } }, body: change });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Retires a plan for good (POST /admin/supporter_plans/{id}/retire), as the options of useMutation. */
export function retireSupporterPlanMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/supporter_plans/{id}/retire"] as const,
    mutationFn: async (id: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.POST("/admin/supporter_plans/{id}/retire", { params: { path: { id } } });
    },
  };
}

/** Ends a subscription at once, without a refund (POST /admin/supporter_subscriptions/{id}/cancel), as the options of useMutation. */
export function cancelSupporterSubscriptionMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/supporter_subscriptions/{id}/cancel"] as const,
    mutationFn: async (id: string): Promise<void> => {
      await api.fetch.POST("/admin/supporter_subscriptions/{id}/cancel", { params: { path: { id } } });
    },
  };
}

/** Queues a reconciliation with the payment provider now (POST /admin/payment_reconciles), as the options of useMutation. */
export function runPaymentReconcileMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/payment_reconciles"] as const,
    mutationFn: async (): Promise<void> => {
      // The middleware throws for every error status (a 429 within the hour); a 204 has no body to read.
      await api.fetch.POST("/admin/payment_reconciles");
    },
  };
}

/** Marks what a reconciliation started by hand changes: the state of the payment provider and the audit log. */
export async function refreshPaymentStatus(queryClient: QueryClient): Promise<void> {
  await Promise.all([queryClient.invalidateQueries({ queryKey: ["get", "/admin/payment_status"] }), refreshAuditLog(queryClient)]);
}

/**
 * Marks what a change of the program, a plan or a subscription changes (ADR-0045): the program (whose count of live
 * subscriptions a change of a subscription changes), the plans (their members), the subscriptions, the roles (whose
 * position and permissions a plan's form changes) and the audit log; the queries shown now are fetched again and
 * awaited.
 */
export async function refreshSupporters(api: Api, queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/supporter_program"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/supporter_plans"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/supporter_subscriptions"] }),
    queryClient.invalidateQueries({ queryKey: rolesQueryOptions(api).queryKey }),
    queryClient.invalidateQueries({ queryKey: ["get", "/instance"] }),
    refreshAuditLog(queryClient),
  ]);
}
