// SPDX-License-Identifier: AGPL-3.0-only
// The server data of the report feature (web-client/README.md S1): the instance rules a report can cite, the
// account's recent posts to choose from, and filing the report (POST /reports, FR-701). Types come from the
// spec (S9).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";

export type Account = components["schemas"]["Account"];
export type Status = components["schemas"]["Status"];
export type InstanceRule = components["schemas"]["InstanceRule"];
export type ReportCategory = components["schemas"]["ReportCategory"];
export type CreateReportRequest = components["schemas"]["CreateReportRequest"];
export type Report = components["schemas"]["Report"];

/** The categories in the order the dialog offers them; `violation` only when the instance has rules. */
export const reportCategories: readonly ReportCategory[] = ["spam", "legal", "violation", "other"];

/** The longest comment the server accepts (CreateReportRequest.comment's maxLength). */
export const maxReportComment = 1000;

/** The most posts and rules one report cites (CreateReportRequest's maxItems). */
export const maxReportItems = 20;

/**
 * The instance, for the rules in effect (`rules`). The key is openapi-react-query's, the same as every
 * other reader of GET /instance, so they share one answer.
 */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/** The account's latest page of posts without its boosts: a report cites only the account's own posts. */
export function reportablePostsQueryOptions(api: Api, accountId: string) {
  return api.query.queryOptions("get", "/accounts/{id}/statuses", {
    params: { path: { id: accountId }, query: { exclude_boosts: true } },
  });
}

/** Files a report (POST /reports), as the options of useMutation. */
export function createReportMutation(api: Api) {
  return {
    mutationKey: ["post", "/reports"] as const,
    mutationFn: async (body: CreateReportRequest): Promise<Report> => {
      const { data, error } = await api.fetch.POST("/reports", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Marks what a new report changes (ADR-0045): for a moderator, the admin screen's lists of reports and its
 * counts of open work, which are cached only once they have opened it. They are marked stale, to be fetched again when they open.
 */
export function cacheFiledReport(queryClient: QueryClient): Promise<void> {
  return queryClient.invalidateQueries({
    predicate: (query) => query.queryKey[1] === "/admin/reports" || query.queryKey[1] === "/admin/counts",
  });
}
