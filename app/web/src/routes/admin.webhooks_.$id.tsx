// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { WebhookDetail, webhookDeliveriesQueryOptions, webhookQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/admin/webhooks`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * One webhook (ADR-0060): its settings, enabling, disabling, testing and deleting it, giving it a new secret, and its
 * deliveries with sending one again, for manage_webhooks.
 */
export const Route = createFileRoute("/admin/webhooks_/$id")({
  staticData: { title: "client:admin.webhook.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_webhooks", () =>
      Promise.all([
        context.queryClient.query({ ...webhookQueryOptions(context.api, id), staleTime: Infinity }),
        context.queryClient.infiniteQuery({ ...webhookDeliveriesQueryOptions(context.api, id), staleTime: Infinity }),
      ]),
    ),
  component: WebhookPage,
  errorComponent: WebhookError,
});

function WebhookHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.webhook.title")}</PageHeading>
      <Link href="/admin/webhooks" variant="standalone" size="sm">
        {t("client:admin.webhook.back")}
      </Link>
    </Stack>
  );
}

function WebhookPage() {
  const { id } = Route.useParams();
  return (
    <>
      <WebhookHeading />
      {/* Another webhook is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <WebhookDetail key={id} id={id} />
    </>
  );
}

function WebhookError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_webhooks" heading={t("client:admin.webhook.title")} />;
}
