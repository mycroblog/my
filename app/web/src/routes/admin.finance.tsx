// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { adminIncomeTotalsQueryOptions, costTotalsQueryOptions, FinanceTotals } from "../features/admin";
import { type CostBasis, type CostGranularity, deviceToday } from "../features/costs";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * `granularity` and `basis` of the totals; omitted, or anything unknown, shows the months amortized
 * (web-client/README.md S3).
 */
const searchSchema = z.object({
  granularity: z.enum(["week", "year"]).optional().catch(undefined),
  basis: z.enum(["cash"]).optional().catch(undefined),
});

type Search = z.infer<typeof searchSchema>;

type TotalsDeps = { granularity: CostGranularity; basis: CostBasis; today: string };

/** The search of the totals, leaving out what is the default. */
function searchOf(totals: { granularity: CostGranularity; basis: CostBasis }): Search {
  return {
    ...(totals.granularity === "month" ? {} : { granularity: totals.granularity }),
    ...(totals.basis === "amortized" ? {} : { basis: totals.basis }),
  };
}

/**
 * The overview of the costs screen (ADR-0067, ADR-0067 §16), for manage_finance: the totals of the costs and of the
 * income by week, month or year, ending with today's period on the viewer's device. The screen's other pages
 * (`/admin/finance/income`, `/costs`, `/imports`, `/publication`) are routes of their own, linked under the heading.
 */
export const Route = createFileRoute("/admin/finance")({
  staticData: { title: "client:admin.finance.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): TotalsDeps => ({
    granularity: search.granularity ?? "month",
    basis: search.basis ?? "amortized",
    today: deviceToday(),
  }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps: { granularity, basis, today } }) =>
    loadWithPermission(context, "manage_finance", () =>
      Promise.all([
        context.queryClient.query({
          ...costTotalsQueryOptions(context.api, granularity, basis, today),
          staleTime: Infinity,
        }),
        context.queryClient.query({
          ...adminIncomeTotalsQueryOptions(context.api, granularity, basis, today),
          staleTime: Infinity,
        }),
      ]),
    ),
  component: FinancePage,
  errorComponent: FinanceError,
});

function FinancePage() {
  const t = useT();
  const { granularity, basis, today } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.finance.title")}</PageHeading>
      <FinanceTotals
        granularity={granularity}
        basis={basis}
        today={today}
        onTotalsChange={(next) => {
          void navigate({ search: searchOf(next) });
        }}
      />
    </>
  );
}

function FinanceError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_finance" heading={t("client:admin.finance.title")} />;
}
