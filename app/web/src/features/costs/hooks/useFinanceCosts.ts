// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { shellQueryRetry } from "../../../lib/query";
import { useViewerPermissions } from "../../moderation";
import {
  type CostBasis,
  type CostGranularity,
  type FinanceCostTotals,
  financeCostTotalsQueryOptions,
} from "../api/costs";

/** The totals of the finances page as it shows them; `ready` says whether the lines of the items are shown. */
export type FinanceCosts =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | { status: "ready"; totals: FinanceCostTotals; items: boolean };

/** Whether the viewer may read the finances page: with view_finance, or with manage_finance, which also reads every item. */
export function useFinanceAccess(): { allowed: boolean; manage: boolean } {
  const permissions = useViewerPermissions();
  const manage = permissions.includes("manage_finance");
  return { allowed: manage || permissions.includes("view_finance"), manage };
}

/**
 * The totals of the shown periods up to `today` for the finances page (GET /finance/cost_totals, ADR-0070 §5), with
 * the lines of the items while the instance shows their names or memos, or always for a holder of manage_finance.
 */
export function useFinanceCosts(granularity: CostGranularity, basis: CostBasis, today: string): FinanceCosts {
  const api = useApi();
  const { allowed, manage } = useFinanceAccess();
  // A viewer without the permission is told why and sends nothing (the server would answer 403).
  const query = useQuery({
    ...financeCostTotalsQueryOptions(api, granularity, basis, today),
    enabled: allowed,
    retry: shellQueryRetry,
    retryOnMount: false,
  });
  if (query.data !== undefined) {
    const { viewer } = query.data;
    return { status: "ready", totals: query.data, items: manage || viewer.item_names || viewer.item_memos };
  }
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

/** The navigation's Finances entry (ADR-0070 §1): whether the viewer holds view_finance. */
export function useFinanceEntry(): { available: boolean } {
  return { available: useViewerPermissions().includes("view_finance") };
}
