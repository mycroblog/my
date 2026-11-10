// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { DomainActionsNav, DomainAllows, domainAllowsQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The allowlist of federation (ADR-0159 §1): the allowed domains, allowing one and removing one, for manage_domains. */
export const Route = createFileRoute("/moderation/domain-actions_/allowed")({
  staticData: { title: "client:admin.domain_allows.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_domains", () =>
      context.queryClient.query({ ...domainAllowsQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: DomainAllowsPage,
  errorComponent: DomainAllowsError,
});

function DomainAllowsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.domain_actions.title")}</PageHeading>
      <DomainActionsNav current="allowed" />
      <DomainAllows />
    </>
  );
}

function DomainAllowsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_domains" heading={t("client:admin.domain_actions.title")} />;
}
