// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the supporter plans as users see them (web-client/README.md S1, ADR-0072 §6, §8, §18,
// ADR-0071 §4): the plans that take subscriptions (GET /instance/supporter_plans, a 404 while the program is
// disabled), the user's own subscription and whether they may subscribe (GET /supporter/subscription), starting a
// subscription at the payment provider's hosted checkout (POST /supporter/checkout), the re-read when the checkout
// or the portal returns (POST /supporter/subscription/sync), changing the plan, cancelling, resuming and withdrawing
// a scheduled change, the provider's portal (POST /supporter/portal), the supporter list (GET /instance/supporters,
// by plan as an infinite query) and the user's choice to be on it (GET / PUT /preferences/supporter_listing).
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";

type Schemas = components["schemas"];

export type SupporterPlan = Schemas["SupporterPlan"];
export type SupporterInterval = Schemas["SupporterInterval"];
type OwnSupporterSubscription = Schemas["OwnSupporterSubscription"];
export type SupporterSubscription = Schemas["SupporterSubscription"];
export type SupporterIneligibleReason = Schemas["SupporterIneligibleReason"];
export type SupporterPlanChoice = Schemas["SupporterPlanChoice"];
export type SupporterListEntry = Schemas["SupporterListEntry"];
export type SupporterListPlan = Schemas["SupporterListPlan"];

/** The intervals of a subscription, in the order of the form. */
export const supporterIntervals = ["month", "year"] as const satisfies readonly SupporterInterval[];

/** The problem types of the subscriptions that the screen tells apart (api/spec Problem). */
export const supporterProblems = {
  ineligible: "/api/v1/problems/supporter-ineligible",
  subscribed: "/api/v1/problems/supporter-subscribed",
  planUnavailable: "/api/v1/problems/supporter-plan-unavailable",
  subscriptionState: "/api/v1/problems/supporter-subscription-state",
  samePlan: "/api/v1/problems/supporter-same-plan",
  subscriptionEnded: "/api/v1/problems/subscription-ended",
} as const;

/** How many members one page of the supporter list holds. */
const listPageSize = 40;

/** The instance, which says whether the program is enabled and where its commerce disclosures are. */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/** The plans that take subscriptions, by rank; a 404 while the program is disabled. */
export function supporterPlansQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance/supporter_plans");
}

/** The user's subscription that goes on and why they may not subscribe; a 404 while the program is disabled. */
export function ownSupporterSubscriptionQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/supporter/subscription");
}

/** The user's choice to be on the supporter list; a 404 for a user who is not a member of a plan. */
export function supporterListingQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/preferences/supporter_listing");
}

/**
 * The members who chose to be listed, by plan in the order of `plans` and newest first within a plan, a page at a
 * time. A 404 when the instance publishes no list, a 401 to a visitor without a session when only signed-in users
 * may read it.
 */
export function supporterListQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/instance/supporters", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = pageParam === undefined ? { limit: listPageSize } : { limit: listPageSize, cursor: pageParam };
      const { data, error } = await api.fetch.GET("/instance/supporters", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Starts a subscription at the provider's hosted checkout (POST /supporter/checkout), as the options of useMutation. */
export function startCheckoutMutation(api: Api) {
  return {
    mutationKey: ["post", "/supporter/checkout"] as const,
    mutationFn: async (choice: SupporterPlanChoice): Promise<Schemas["SupporterCheckout"]> => {
      const { data, error } = await api.fetch.POST("/supporter/checkout", { body: choice });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Opens the provider's portal of the user's payment methods and invoices (POST /supporter/portal), as the options of useMutation. */
export function openPortalMutation(api: Api) {
  return {
    mutationKey: ["post", "/supporter/portal"] as const,
    mutationFn: async (): Promise<Schemas["SupporterPortal"]> => {
      const { data, error } = await api.fetch.POST("/supporter/portal", {});
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** A change of the user's subscription: a new plan or interval, cancelling, resuming, or withdrawing the scheduled change. */
export type SubscriptionCommand =
  | { kind: "change"; choice: SupporterPlanChoice }
  | { kind: "cancel" }
  | { kind: "resume" }
  | { kind: "cancel_scheduled_change" };

/** Sends a change of the user's subscription, as the options of useMutation. Returns the subscription after it. */
export function subscriptionCommandMutation(api: Api) {
  return {
    mutationKey: ["post", "/supporter/subscription"] as const,
    mutationFn: async (command: SubscriptionCommand): Promise<SupporterSubscription> => {
      const { data, error } =
        command.kind === "change"
          ? await api.fetch.POST("/supporter/subscription/change", { body: command.choice })
          : command.kind === "cancel"
            ? await api.fetch.POST("/supporter/subscription/cancel", {})
            : command.kind === "resume"
              ? await api.fetch.POST("/supporter/subscription/resume", {})
              : await api.fetch.POST("/supporter/subscription/cancel_scheduled_change", {});
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Puts the subscription as the server answered a change into the user's subscription (ADR-0045), and marks what the
 * change may move: the user's choice to be listed and the supporter list (a member who stops being one leaves it),
 * and the viewer's account (the plan's role and its badge).
 */
export async function cacheSubscription(api: Api, queryClient: QueryClient, subscription: SupporterSubscription): Promise<void> {
  queryClient.setQueryData(ownSupporterSubscriptionQueryOptions(api).queryKey, (own: OwnSupporterSubscription | undefined) => ({ ...own, subscription }));
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: supporterListingQueryOptions(api).queryKey }),
    queryClient.invalidateQueries({ queryKey: ["get", "/instance/supporters"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/accounts/verify_credentials"] }),
  ]);
}

/**
 * Asks the server to re-read the user's subscription from the provider (POST /supporter/subscription/sync), which
 * the screen does once when the provider's checkout or portal returns to it, and puts the subscription as it is now
 * into the cache; the screen reads it again while it is pending.
 */
export async function syncOwnSubscription(api: Api, queryClient: QueryClient): Promise<void> {
  const { data, error } = await api.fetch.POST("/supporter/subscription/sync", {});
  if (data === undefined) throw new ApiError(error);
  queryClient.setQueryData(ownSupporterSubscriptionQueryOptions(api).queryKey, data);
}

/** Lists the user on the supporter list or takes them off it (PUT /preferences/supporter_listing), as the options of useMutation. */
export function updateListingMutation(api: Api) {
  return {
    mutationKey: ["put", "/preferences/supporter_listing"] as const,
    mutationFn: async (listed: boolean): Promise<Schemas["SupporterListing"]> => {
      const { data, error } = await api.fetch.PUT("/preferences/supporter_listing", { body: { listed } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Puts the choice as the server answered it into the cache, and marks the supporter list, which it changes (ADR-0045). */
export async function cacheListing(api: Api, queryClient: QueryClient, saved: Schemas["SupporterListing"]): Promise<void> {
  queryClient.setQueryData(supporterListingQueryOptions(api).queryKey, saved);
  await queryClient.invalidateQueries({ queryKey: ["get", "/instance/supporters"] });
}
