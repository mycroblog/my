// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { DomainActionsNav, DomainBlockCsvPage } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The domain actions in Mastodon's domain block CSV (FR-706): exporting them and importing a file, for manage_domains. */
export const Route = createFileRoute("/moderation/domain-actions_/import")({
  staticData: { title: "client:admin.domain_actions.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => loadWithPermission(context, "manage_domains", () => Promise.resolve()),
  component: DomainBlockCsvRoute,
  errorComponent: DomainBlockCsvError,
});

function DomainBlockCsvRoute() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.domain_actions.title")}</PageHeading>
      <DomainActionsNav current="import" />
      <DomainBlockCsvPage />
    </>
  );
}

function DomainBlockCsvError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_domains" heading={t("client:admin.domain_actions.title")} />;
}
