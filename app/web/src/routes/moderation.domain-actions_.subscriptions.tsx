// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { DomainActionsNav, DomainListSubscriptions, domainListSubscriptionsQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The subscriptions to the domain blocklists that others publish (ADR-0159 §5), newest first, for manage_domains; each
 * row links to its page. The `_` keeps it out of the list of domain actions' layout.
 */
export const Route = createFileRoute("/moderation/domain-actions_/subscriptions")({
  staticData: { title: "client:admin.domain_lists.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_domains", () =>
      context.queryClient.infiniteQuery({
        ...domainListSubscriptionsQueryOptions(context.api),
        staleTime: Infinity,
      }),
    ),
  component: DomainListSubscriptionsPage,
  errorComponent: DomainListSubscriptionsError,
});

function DomainListSubscriptionsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.domain_actions.title")}</PageHeading>
      <DomainActionsNav current="subscriptions" />
      <DomainListSubscriptions />
    </>
  );
}

function DomainListSubscriptionsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_domains" heading={t("client:admin.domain_actions.title")} />;
}
