// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  adminIncomeTotalsQueryOptions,
  billingImportsQueryOptions,
  costItemsQueryOptions,
  costTotalsQueryOptions,
  FinanceSettings,
  type FinanceTab,
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
 * `tab`: recording the costs (the recorded items and the billing imports); omitted, or anything unknown, shows the
 * overview. `granularity` and `basis` of the totals; omitted, or anything unknown, shows the months amortized. `kind`,
 * `from` and `to` (`YYYY-MM-DD`) narrow the money movements; omitted, or anything unknown, narrows nothing
 * (web-client/README.md S3).
 */
const searchSchema = z.object({
  tab: z.literal("records").optional().catch(undefined),
  granularity: z.enum(["week", "year"]).optional().catch(undefined),
  basis: z.enum(["cash"]).optional().catch(undefined),
  kind: z.enum(incomeKinds).optional().catch(undefined),
  from: z.iso.date().optional().catch(undefined),
  to: z.iso.date().optional().catch(undefined),
});

type Search = z.infer<typeof searchSchema>;

type FinanceDeps = {
  tab: FinanceTab;
  granularity: CostGranularity;
  basis: CostBasis;
  today: string;
  movements: IncomeMovementFilter;
};

/** The search of the overview's totals and of the movements' filter, leaving out what is the default. */
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
 * The instance's costs and income (ADR-0067, ADR-0068, ADR-0073 §6), for manage_finance, in two tabs: reading them (the
 * totals of both by week, month or year, what is published, and the money movements of the income, of a kind and a
 * range of dates) and recording them (the recorded items and the state of the billing imports). The totals end with
 * today's period on the viewer's device. Each tab loads only what it shows.
 */
export const Route = createFileRoute("/admin/finance")({
  staticData: { title: "client:admin.finance.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): FinanceDeps => ({
    tab: search.tab ?? "overview",
    granularity: search.granularity ?? "month",
    basis: search.basis ?? "amortized",
    today: deviceToday(),
    movements: { kind: search.kind, from: search.from, to: search.to },
  }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps: { tab, granularity, basis, today, movements } }) =>
    loadWithPermission(context, "manage_finance", () =>
      tab === "records"
        ? Promise.all([
            context.queryClient.infiniteQuery({ ...costItemsQueryOptions(context.api), staleTime: Infinity }),
            context.queryClient.query({ ...billingImportsQueryOptions(context.api), staleTime: Infinity }),
          ])
        : Promise.all([
            context.queryClient.query({
              ...costTotalsQueryOptions(context.api, granularity, basis, today),
              staleTime: Infinity,
            }),
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
  const { tab, granularity, basis, today, movements } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.finance.title")}</PageHeading>
      <FinanceSettings
        tab={tab}
        onTabChange={(next) => {
          // The overview's search stays in the URL while the records are shown, so that coming back finds it again.
          void navigate({
            search: { ...searchOf({ granularity, basis }, movements), ...(next === "records" && { tab: "records" }) },
          });
        }}
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
