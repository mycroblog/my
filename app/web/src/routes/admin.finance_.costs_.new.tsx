// SPDX-License-Identifier: AGPL-3.0-only
import { BackLinkHeading } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewCostItem } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * Recording a cost (ADR-0067), for manage_finance: the form only, which reads nothing from the server. The `_`s keep
 * the page out of the layouts of the overview and of the list.
 */
export const Route = createFileRoute("/admin/finance_/costs_/new")({
  staticData: { title: "client:admin.finance.new.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => loadWithPermission(context, "manage_finance", () => Promise.resolve()),
  component: NewCostPage,
  errorComponent: NewCostError,
});

function NewCostPage() {
  const t = useT();
  return (
    <>
      <BackLinkHeading backHref="/admin/finance/costs" backLabel={t("client:admin.finance.item.back")}>
        <PageHeading>{t("client:admin.finance.new.title")}</PageHeading>
      </BackLinkHeading>
      <NewCostItem />
    </>
  );
}

function NewCostError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_finance" heading={t("client:admin.finance.new.title")} />;
}
