// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  adminIncomeTotalsQueryOptions,
  billingImportsQueryOptions,
  costItemsQueryOptions,
  costTotalsQueryOptions,
  FinanceSettings,
  type IncomeMovementFilter,
  incomeKinds,
  incomeMovementsQueryOptions,
} from "../features/admin";
import { type CostBasis, type CostGranularity, deviceToday } from "../features/costs";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * `granularity` and `basis` of the totals; omitted, or anything unknown, shows the months amortized. `kind`, `from`
 * and `to` (`YYYY-MM-DD`) narrow the money movements; omitted, or anything unknown, narrows nothing (web-client/README.md S3).
 */
const searchSchema = z.object({
  granularity: z.enum(["week", "year"]).optional().catch(undefined),
  basis: z.enum(["cash"]).optional().catch(undefined),
  kind: z.enum(incomeKinds).optional().catch(undefined),
  from: z.iso.date().optional().catch(undefined),
  to: z.iso.date().optional().catch(undefined),
});

type Search = z.infer<typeof searchSchema>;

type FinanceDeps = { granularity: CostGranularity; basis: CostBasis; today: string; movements: IncomeMovementFilter };

/** The search of the totals and of the movements' filter, leaving out what is the default. */
function searchOf(totals: { granularity: CostGranularity; basis: CostBasis }, movements: IncomeMovementFilter): Search {
  return {
    ...(totals.granularity === "month" ? {} : { granularity: totals.granularity }),
    ...(totals.basis === "amortized" ? {} : { basis: totals.basis }),
    ...(movements.kind === undefined ? {} : { kind: movements.kind }),
    ...(movements.from === undefined ? {} : { from: movements.from }),
    ...(movements.to === undefined ? {} : { to: movements.to }),
  };
}

/**
 * The instance's costs and income (ADR-0067, ADR-0068, ADR-0073 §6): recording the costs, the totals of both by week,
 * month or year, the state of the billing imports, and the money movements of the income, of a kind and a range of
 * dates, for manage_finance. The totals end with today's period on the viewer's device.
 */
export const Route = createFileRoute("/admin/finance")({
  staticData: { title: "client:admin.finance.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): FinanceDeps => ({
    granularity: search.granularity ?? "month",
    basis: search.basis ?? "amortized",
    today: deviceToday(),
    movements: { kind: search.kind, from: search.from, to: search.to },
  }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps: { granularity, basis, today, movements } }) =>
    loadWithPermission(context, "manage_finance", () =>
      Promise.all([
        context.queryClient.query({
          ...costTotalsQueryOptions(context.api, granularity, basis, today),
          staleTime: Infinity,
        }),
        context.queryClient.infiniteQuery({ ...costItemsQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.query({ ...billingImportsQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.query({
          ...adminIncomeTotalsQueryOptions(context.api, granularity, basis, today),
          staleTime: Infinity,
        }),
        context.queryClient.infiniteQuery({
          ...incomeMovementsQueryOptions(context.api, movements),
          staleTime: Infinity,
        }),
      ]),
    ),
  component: FinancePage,
  errorComponent: FinanceError,
});

function FinancePage() {
  const t = useT();
  const { granularity, basis, today, movements } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.finance.title")}</PageHeading>
      <FinanceSettings
        granularity={granularity}
        basis={basis}
        today={today}
        onTotalsChange={(next) => {
          void navigate({ search: searchOf(next, movements) });
        }}
        movements={movements}
        onMovementsChange={(next) => {
          void navigate({ search: searchOf({ granularity, basis }, next) });
        }}
      />
    </>
  );
}

function FinanceError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_finance" heading={t("client:admin.finance.title")} />;
}
