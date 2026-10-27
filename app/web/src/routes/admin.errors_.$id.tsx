// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { ErrorGroupDetail, errorGroupQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/admin/errors`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * One error group (ADR-0103 §8): its state with resolving and muting it, its latest occurrence, its hourly counts and
 * the changes of its state, for manage_errors. An `admin_error_group` notification leads here.
 */
export const Route = createFileRoute("/admin/errors_/$id")({
  staticData: { title: "client:admin.errors.detail_title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_errors", () => context.queryClient.query({ ...errorGroupQueryOptions(context.api, id), staleTime: Infinity })),
  component: ErrorGroupPage,
  errorComponent: ErrorGroupError,
});

function ErrorGroupHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.errors.detail_title")}</PageHeading>
      <Link href="/admin/errors" variant="standalone" size="sm">
        {t("client:admin.errors.back")}
      </Link>
    </Stack>
  );
}

function ErrorGroupPage() {
  const { id } = Route.useParams();
  return (
    <>
      <ErrorGroupHeading />
      {/* Another group is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <ErrorGroupDetail key={id} id={id} />
    </>
  );
}

function ErrorGroupError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_errors" heading={t("client:admin.errors.detail_title")} />;
}
