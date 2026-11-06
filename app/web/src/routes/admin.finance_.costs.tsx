// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { CostItems, costItemsQueryOptions } from "../features/admin";
import { deviceToday } from "../features/costs";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The recorded costs of the costs screen (ADR-0067), for manage_finance: the items that are not canceled, newest
 * first, with their prices on today's date on the viewer's device, and "Record a cost". The `_` keeps the page out
 * of the overview's layout.
 */
export const Route = createFileRoute("/admin/finance_/costs")({
  staticData: { title: "client:admin.finance.title" },
  loaderDeps: () => ({ today: deviceToday() }),
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_finance", () =>
      context.queryClient.infiniteQuery({ ...costItemsQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: CostsPage,
  errorComponent: FinanceError,
});

function CostsPage() {
  const t = useT();
  const { today } = Route.useLoaderDeps();
  return (
    <>
      <PageHeading>{t("client:admin.finance.title")}</PageHeading>
      <CostItems today={today} />
    </>
  );
}

function FinanceError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_finance" heading={t("client:admin.finance.title")} />;
}
