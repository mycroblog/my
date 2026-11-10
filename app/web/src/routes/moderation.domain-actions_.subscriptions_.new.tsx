// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewDomainListSubscription } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The form of a new subscription to a domain blocklist (ADR-0159 §4, §5), for manage_domains. */
export const Route = createFileRoute("/moderation/domain-actions_/subscriptions_/new")({
  staticData: { title: "client:admin.domain_lists.new.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => loadWithPermission(context, "manage_domains", () => Promise.resolve()),
  component: NewSubscriptionPage,
  errorComponent: NewSubscriptionError,
});

function NewSubscriptionPage() {
  const t = useT();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.domain_lists.new.title")}</PageHeading>
        <Link href="/moderation/domain-actions/subscriptions" variant="standalone" size="sm">
          {t("client:admin.domain_lists.back")}
        </Link>
      </Stack>
      <NewDomainListSubscription />
    </>
  );
}

function NewSubscriptionError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_domains" heading={t("client:admin.domain_lists.new.title")} />;
}
