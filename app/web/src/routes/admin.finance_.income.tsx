// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { FinanceIncome, type IncomeMovementFilter, incomeKinds, incomeMovementsQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * `kind`, `from` and `to` (`YYYY-MM-DD`) narrow the money movements; omitted, or anything unknown, narrows nothing
 * (web-client/README.md S3).
 */
const searchSchema = z.object({
  kind: z.enum(incomeKinds).optional().catch(undefined),
  from: z.iso.date().optional().catch(undefined),
  to: z.iso.date().optional().catch(undefined),
});

type Search = z.infer<typeof searchSchema>;

/** The search of the movements' filter, leaving out what narrows nothing. */
function searchOf(filter: IncomeMovementFilter): Search {
  return {
    ...(filter.kind === undefined ? {} : { kind: filter.kind }),
    ...(filter.from === undefined ? {} : { from: filter.from }),
    ...(filter.to === undefined ? {} : { to: filter.to }),
  };
}

/**
 * The payments and refunds of the costs screen (ADR-0067 §16), for manage_finance: the payment provider's money
 * movements of the income, of a kind and a range of dates. The `_` keeps the page out of the overview's layout.
 */
export const Route = createFileRoute("/admin/finance_/income")({
  staticData: { title: "client:admin.finance.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): IncomeMovementFilter => ({ kind: search.kind, from: search.from, to: search.to }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps }) =>
    loadWithPermission(context, "manage_finance", () =>
      context.queryClient.infiniteQuery({ ...incomeMovementsQueryOptions(context.api, deps), staleTime: Infinity }),
    ),
  component: IncomePage,
  errorComponent: FinanceError,
});

function IncomePage() {
  const t = useT();
  const filter = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.finance.title")}</PageHeading>
      <FinanceIncome
        filter={filter}
        onFilterChange={(next) => {
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
