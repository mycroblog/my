// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  type CostBasis,
  type CostGranularity,
  deviceToday,
  FinanceCosts,
  financeCostTotalsQueryOptions,
  financeIncomeTotalsQueryOptions,
} from "../features/costs";
import { viewerQueryOptions } from "../features/session";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/**
 * `granularity` and `basis` of the totals; omitted, or anything unknown, shows the months amortized (ADR-0067 §4,
 * web-client/README.md S3).
 */
const searchSchema = z.object({
  granularity: z.enum(["week", "year"]).optional().catch(undefined),
  basis: z.enum(["cash"]).optional().catch(undefined),
});

/**
 * The instance's finances for the holders of view_finance (ADR-0070 §5, ADR-0073 §6): the totals of its costs and of
 * its income by week, month or year, with as much of the items and of the plans as the instance shows. It is a page of the user's side, not of the staff's screens,
 * so a viewer who is not staff needs no admin scope. The totals end with today's period on the viewer's device.
 */
export const Route = createFileRoute("/finance")({
  staticData: { title: "client:finance.title", hideHeading: true },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): { granularity: CostGranularity; basis: CostBasis; today: string } => ({
    granularity: search.granularity ?? "month",
    basis: search.basis ?? "amortized",
    today: deviceToday(),
  }),
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api }, deps: { granularity, basis, today } }) => {
    // The viewer's permissions decide whether the page reads anything; a viewer without them is told why, and the
    // totals' own failures show in the page (useFinanceCosts). Cached data is shown at once (web-client/README.md S7).
    const viewer = await queryClient.query(viewerQueryOptions(api));
    if (!viewer.permissions.includes("view_finance") && !viewer.permissions.includes("manage_finance")) return;
    await Promise.all([
      queryClient
        .query({ ...financeCostTotalsQueryOptions(api, granularity, basis, today), staleTime: Infinity })
        .catch(() => undefined),
      queryClient
        .query({ ...financeIncomeTotalsQueryOptions(api, granularity, basis, today), staleTime: Infinity })
        .catch(() => undefined),
    ]);
  },
  component: FinancePage,
  errorComponent: FinanceError,
});

function FinancePage() {
  const t = useT();
  const { granularity, basis, today } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:finance.title")}</PageHeading>
      <FinanceCosts
        granularity={granularity}
        basis={basis}
        today={today}
        onTotalsChange={(next) => {
          void navigate({
            search: {
              ...(next.granularity === "month" ? {} : { granularity: next.granularity }),
              ...(next.basis === "amortized" ? {} : { basis: next.basis }),
            },
          });
        }}
      />
    </>
  );
}

function FinanceError(props: ErrorComponentProps) {
  const t = useT();
  return <RouteError {...props} heading={t("client:finance.title")} />;
}
