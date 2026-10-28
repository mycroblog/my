// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { shellQueryRetry } from "../../../lib/query";
import type { CostBasis, CostGranularity } from "../api/costs";
import { financeIncomeTotalsQueryOptions, type IncomeTotals, publicIncomeTotalsQueryOptions } from "../api/income";
import { useFinanceAccess } from "./useFinanceCosts";

/** The income totals as a page shows them. `hidden` is the instance setting that publishes nothing (404). */
export type Income =
  | { status: "hidden" }
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | { status: "ready"; totals: IncomeTotals };

/**
 * The income totals that the instance publishes for the shown periods up to `today` (FR-917, ADR-0073 §7). A 4xx is
 * the instance's setting or a query the page never makes, not a failure to try again, so it is not retried.
 */
export function usePublicIncome(granularity: CostGranularity, basis: CostBasis, today: string): Income {
  const api = useApi();
  const query = useQuery({
    ...publicIncomeTotalsQueryOptions(api, granularity, basis, today),
    retry: shellQueryRetry,
    retryOnMount: false,
  });
  if (query.data !== undefined) return { status: "ready", totals: query.data };
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

/**
 * The income totals of the shown periods up to `today` for the finances page (GET /finance/income_totals,
 * ADR-0073 §6): the net and the balance always, the members of each plan when the instance shows them. A viewer
 * without the permission sends nothing.
 */
export function useFinanceIncome(
  granularity: CostGranularity,
  basis: CostBasis,
  today: string,
): Exclude<Income, { status: "hidden" }> {
  const api = useApi();
  const { allowed } = useFinanceAccess();
  const query = useQuery({
    ...financeIncomeTotalsQueryOptions(api, granularity, basis, today),
    enabled: allowed,
    retry: shellQueryRetry,
    retryOnMount: false,
  });
  if (query.data !== undefined) return { status: "ready", totals: query.data };
  if (query.error !== null) {
    return {
      status: "error",
      reason: errorMessage(query.error),
      retry: () => void query.refetch(),
      isRetrying: query.isFetching,
    };
  }
  return { status: "loading" };
}
