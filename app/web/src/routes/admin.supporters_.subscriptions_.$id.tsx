// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { SupporterSubscriptionDetail, supporterSubscriptionQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/admin/supporters/subscriptions`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** One subscription to a supporter plan (ADR-0072 §10): its facts and ending it at once, for manage_supporters. */
export const Route = createFileRoute("/admin/supporters_/subscriptions_/$id")({
  staticData: { title: "client:admin.supporters.subscription.page_title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_supporters", () =>
      context.queryClient.query({ ...supporterSubscriptionQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: SubscriptionPage,
  errorComponent: SubscriptionError,
});

function SubscriptionPage() {
  const t = useT();
  const { id } = Route.useParams();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.supporters.subscription.page_title")}</PageHeading>
        <Link href="/admin/supporters/subscriptions" variant="standalone" size="sm">
          {t("client:admin.supporters.subscription.back")}
        </Link>
      </Stack>
      {/* Another subscription is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <SupporterSubscriptionDetail key={id} id={id} />
    </>
  );
}

function SubscriptionError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError
      {...props}
      permission="manage_supporters"
      heading={t("client:admin.supporters.subscription.page_title")}
    />
  );
}
