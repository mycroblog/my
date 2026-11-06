// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { BillingImports, billingImportsQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The billing imports of the costs screen (ADR-0068), for manage_finance: the state of each billing source and
 * importing its charges now. The `_` keeps the page out of the overview's layout.
 */
export const Route = createFileRoute("/admin/finance_/imports")({
  staticData: { title: "client:admin.finance.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_finance", () =>
      context.queryClient.query({ ...billingImportsQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: ImportsPage,
  errorComponent: FinanceError,
});

function ImportsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.finance.title")}</PageHeading>
      <BillingImports />
    </>
  );
}

function FinanceError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_finance" heading={t("client:admin.finance.title")} />;
}
