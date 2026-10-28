// SPDX-License-Identifier: AGPL-3.0-only
import { Card, Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { AppealDetailView, appealQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/moderation/appeals`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** One appeal next to its action, and upholding or overturning it (FR-707), for manage_accounts. */
export const Route = createFileRoute("/moderation/appeals_/$id")({
  staticData: { title: "client:admin.appeal.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_accounts", () =>
      context.queryClient.query({ ...appealQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: AppealPage,
  errorComponent: AppealError,
});

function AppealHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.appeal.title")}</PageHeading>
      <Link href="/moderation/appeals" variant="standalone" size="sm">
        {t("client:admin.appeal.back")}
      </Link>
    </Stack>
  );
}

function AppealPage() {
  const { id } = Route.useParams();
  return (
    <>
      <AppealHeading />
      {/* Another appeal is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <Card>
        <AppealDetailView key={id} id={id} />
      </Card>
    </>
  );
}

function AppealError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.appeal.title")} />;
}
