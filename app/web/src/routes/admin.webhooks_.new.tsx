// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewWebhook } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * Setting up a webhook (ADR-0060, FR-722), for manage_webhooks; its secret is shown once on this page, with the link
 * to the webhook's page. The `_` keeps the page out of the list's layout at `/admin/webhooks`.
 */
export const Route = createFileRoute("/admin/webhooks_/new")({
  staticData: { title: "client:admin.webhooks.new.title" },
  beforeLoad: requireSignIn,
  component: NewWebhookPage,
  errorComponent: NewWebhookError,
});

function NewWebhookPage() {
  const t = useT();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.webhooks.new.title")}</PageHeading>
        <Link href="/admin/webhooks" variant="standalone" size="sm">
          {t("client:admin.webhook.back")}
        </Link>
      </Stack>
      <NewWebhook />
    </>
  );
}

function NewWebhookError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_webhooks" heading={t("client:admin.webhooks.new.title")} />;
}
