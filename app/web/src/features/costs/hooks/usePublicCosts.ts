// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { shellQueryRetry } from "../../../lib/query";
import { type CostBasis, type CostGranularity, type CostTotals, publicCostTotalsQueryOptions } from "../api/costs";

/**
 * The public totals as the page shows them. `hidden` is the instance setting that publishes nothing (404); `ready`
 * says whether the instance publishes the breakdown by provider and category with them.
 */
export type PublicCosts =
  | { status: "hidden" }
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | { status: "ready"; totals: CostTotals; breakdown: boolean };

/**
 * The totals that the instance publishes for the shown periods up to `today` (FR-903). `breakdown` is the instance's
 * `finance_visibility` from GET /instance: the totals alone carry no sign of it. A 4xx is the instance's setting or a
 * query the page never makes, not a failure to try again, so it is not retried.
 */
export function usePublicCosts(
  granularity: CostGranularity,
  basis: CostBasis,
  today: string,
  breakdown: boolean,
): PublicCosts {
  const api = useApi();
  const query = useQuery({
    ...publicCostTotalsQueryOptions(api, granularity, basis, today),
    retry: shellQueryRetry,
    retryOnMount: false,
  });
  if (query.data !== undefined) return { status: "ready", totals: query.data, breakdown };
  if (query.error !== null) {
    if (query.error instanceof ApiError && query.error.status === 404) return { status: "hidden" };
    return {
      status: "error",
      reason: errorMessage(query.error),
      retry: () => void query.refetch(),
      isRetrying: query.isFetching,
    };
  }
  return { status: "loading" };
}
