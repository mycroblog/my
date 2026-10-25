// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the costs screen (ADR-0067 §9, ADR-0068 §9, manage_finance, web-client/README.md S1): the
// cost items (GET /admin/cost_items, newest first, as an infinite query), the totals of a range of weeks, months or
// years under a basis (GET /admin/cost_totals), the state of the billing imports (GET /admin/billing_imports),
// recording, relabeling, revising, ending and canceling an item, and importing a provider's charges now. Every change
// goes to the audit log and changes the totals. The income (ADR-0073 §6) is only read: its totals with the gross, the
// fees and the balance (GET /admin/income_totals), and the payment provider's money movements, newest first, with their
// subscribers (GET /admin/income_movements); the payment provider records it, and nobody changes it here.
import { infiniteQueryOptions, type QueryClient, queryOptions } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { type CostBasis, type CostGranularity, type Money, totalsQuery } from "../../costs";
import { pageQuery, refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type CostItem = Schemas["CostItem"];
export type CostSchedule = Schemas["CostSchedule"];
export type NewCostItem = Schemas["CostItemCreateRequest"];
export type CostItemLabel = Schemas["CostItemLabelRequest"];
export type BillingImport = Schemas["BillingImport"];
export type AdminIncomeTotals = Schemas["AdminIncomeTotals"];
export type IncomeMovement = Schemas["IncomeMovement"];
export type IncomeKind = Schemas["IncomeKind"];

/** The schedules, in the order of the form. */
export const costSchedules = ["once", "weeks", "months", "years"] as const satisfies readonly CostSchedule[];

/** The most N of an every-N schedule, and the longest name, provider and memo (CostItemCreateRequest). */
export const maxCostInterval = 99;
export const costNameLimit = 100;
export const costMemoLimit = 10_000;

/** The price of an item's payment on `date`: the revision with the latest effective date on or before it, the last recorded of a date. */
export function priceOn(item: CostItem, date: string): Money | undefined {
  let best: Schemas["CostPriceRevision"] | undefined;
  for (const revision of item.prices) {
    if (revision.effective_on > date) continue;
    if (best === undefined || revision.effective_on >= best.effective_on) best = revision;
  }
  return best?.price;
}

/** The cost items that are not canceled, newest first (GET /admin/cost_items). */
export function costItemsQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/cost_items", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/cost_items", { params: { query: pageQuery(pageParam) }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The totals of the shown periods up to `today` (GET /admin/cost_totals). */
export function costTotalsQueryOptions(api: Api, granularity: CostGranularity, basis: CostBasis, today: string) {
  const query = totalsQuery(granularity, basis, today);
  return queryOptions({
    queryKey: ["get", "/admin/cost_totals", query] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.GET("/admin/cost_totals", { params: { query }, signal });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  });
}

/** The income totals of the shown periods up to `today`, with the gross, the fees and the balance (GET /admin/income_totals). */
export function adminIncomeTotalsQueryOptions(api: Api, granularity: CostGranularity, basis: CostBasis, today: string) {
  const query = totalsQuery(granularity, basis, today);
  return queryOptions({
    queryKey: ["get", "/admin/income_totals", query] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.GET("/admin/income_totals", { params: { query }, signal });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  });
}

/** The kinds of the money movements, in the order of the filter. */
export const incomeKinds = ["payment", "refund", "chargeback", "chargeback_reversal", "fee"] as const satisfies readonly IncomeKind[];

/**
 * The money movements shown: of a kind, and counted on or after `from` and on or before `to` (`YYYY-MM-DD`, the dates
 * in the provider's reporting time zone); undefined narrows nothing.
 */
export type IncomeMovementFilter = { kind?: IncomeKind | undefined; from?: string | undefined; to?: string | undefined };

/** The payment provider's money movements of the income, newest first, narrowed by the filter (GET /admin/income_movements). */
export function incomeMovementsQueryOptions(api: Api, filter: IncomeMovementFilter) {
  const narrow = {
    ...(filter.kind === undefined ? {} : { kind: filter.kind }),
    ...(filter.from === undefined ? {} : { from: filter.from }),
    ...(filter.to === undefined ? {} : { to: filter.to }),
  };
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/income_movements", narrow, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/income_movements", { params: { query: { ...pageQuery(pageParam), ...narrow } }, signal });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The state of the imports of every billing source (GET /admin/billing_imports). */
export function billingImportsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/billing_imports");
}

/** Records a cost item (POST /admin/cost_items), as the options of useMutation. */
export function createCostItemMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/cost_items"] as const,
    mutationFn: async (body: NewCostItem): Promise<CostItem> => {
      const { data, error } = await api.fetch.POST("/admin/cost_items", { body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Changes the label of a cost item (PATCH /admin/cost_items/{id}). */
export function updateCostItemMutation(api: Api) {
  return {
    mutationKey: ["patch", "/admin/cost_items/{id}"] as const,
    mutationFn: async ({ id, body }: { id: string; body: CostItemLabel }): Promise<CostItem> => {
      const { data, error } = await api.fetch.PATCH("/admin/cost_items/{id}", { params: { path: { id } }, body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Revises the price of a cost item from a date on (POST /admin/cost_items/{id}/price_revisions). */
export function reviseCostItemPriceMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/cost_items/{id}/price_revisions"] as const,
    mutationFn: async ({ id, body }: { id: string; body: Schemas["CostPriceRevisionRequest"] }): Promise<CostItem> => {
      const { data, error } = await api.fetch.POST("/admin/cost_items/{id}/price_revisions", { params: { path: { id } }, body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Sets the end of a cost item, or none without `ends_on` (PUT /admin/cost_items/{id}/end). */
export function setCostItemEndMutation(api: Api) {
  return {
    mutationKey: ["put", "/admin/cost_items/{id}/end"] as const,
    mutationFn: async ({ id, endsOn }: { id: string; endsOn: string | undefined }): Promise<CostItem> => {
      const body = endsOn === undefined ? {} : { ends_on: endsOn };
      const { data, error } = await api.fetch.PUT("/admin/cost_items/{id}/end", { params: { path: { id } }, body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Cancels a cost item for good (POST /admin/cost_items/{id}/cancel). */
export function cancelCostItemMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/cost_items/{id}/cancel"] as const,
    mutationFn: async (id: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.POST("/admin/cost_items/{id}/cancel", { params: { path: { id } } });
    },
  };
}

/** Queues an import of a billing source now (POST /admin/billing_imports/{source}/runs). */
export function runBillingImportMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/billing_imports/{source}/runs"] as const,
    mutationFn: async (source: Schemas["BillingSourceName"]): Promise<void> => {
      await api.fetch.POST("/admin/billing_imports/{source}/runs", { params: { path: { source } } });
    },
  };
}

/**
 * Marks what a change of the cost items changes (ADR-0045): the list, every range of totals and the audit log; the
 * queries shown now are fetched again and awaited.
 */
export async function refreshCosts(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/cost_items", "pages"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/cost_totals"] }),
    refreshAuditLog(queryClient),
  ]);
}

/** Marks what an import started by hand changes: the state of the imports and the audit log. */
export async function refreshBillingImports(queryClient: QueryClient): Promise<void> {
  await Promise.all([queryClient.invalidateQueries({ queryKey: ["get", "/admin/billing_imports"] }), refreshAuditLog(queryClient)]);
}
