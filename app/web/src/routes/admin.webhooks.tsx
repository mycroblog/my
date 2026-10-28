// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { WebhookSettings, webhooksQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The server's webhooks (ADR-0060, FR-722): setting them up, changing, testing and deleting them, for manage_webhooks. */
export const Route = createFileRoute("/admin/webhooks")({
  staticData: { title: "client:admin.webhooks.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_webhooks", () =>
      context.queryClient.query({ ...webhooksQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: WebhooksPage,
  errorComponent: WebhooksError,
});

function WebhooksPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.webhooks.title")}</PageHeading>
      <WebhookSettings />
    </>
  );
}

function WebhooksError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_webhooks" heading={t("client:admin.webhooks.title")} />;
}
