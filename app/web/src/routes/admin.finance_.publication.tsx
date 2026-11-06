// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { FinancePublication } from "../features/admin";
import { deviceToday } from "../features/costs";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * What the instance publishes of the costs and of the income (ADR-0067 §8, §17), for manage_finance: its form for a
 * viewer who also has manage_settings, what it is now for the others. The form reads the server settings itself, so
 * the loader reads nothing more than the permission. The `_` keeps the page out of the overview's layout.
 */
export const Route = createFileRoute("/admin/finance_/publication")({
  staticData: { title: "client:admin.finance.title" },
  loaderDeps: () => ({ today: deviceToday() }),
  beforeLoad: requireSignIn,
  loader: ({ context }) => loadWithPermission(context, "manage_finance", () => Promise.resolve()),
  component: PublicationPage,
  errorComponent: FinanceError,
});

function PublicationPage() {
  const t = useT();
  const { today } = Route.useLoaderDeps();
  return (
    <>
      <PageHeading>{t("client:admin.finance.title")}</PageHeading>
      <FinancePublication today={today} />
    </>
  );
}

function FinanceError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_finance" heading={t("client:admin.finance.title")} />;
}
