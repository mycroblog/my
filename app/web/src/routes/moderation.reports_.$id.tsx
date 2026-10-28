// SPDX-License-Identifier: AGPL-3.0-only
import { Card, Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { ReportDetailView, reportQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the queue's layout at `/moderation/reports`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** One report, and handling it on one page (FR-710, NFR-MOD-4), for manage_reports. */
export const Route = createFileRoute("/moderation/reports_/$id")({
  staticData: { title: "client:admin.report.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_reports", () =>
      context.queryClient.query({ ...reportQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: ReportPage,
  errorComponent: ReportError,
});

function ReportHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.report.title")}</PageHeading>
      <Link href="/moderation/reports" variant="standalone" size="sm">
        {t("client:admin.report.back")}
      </Link>
    </Stack>
  );
}

function ReportPage() {
  const { id } = Route.useParams();
  return (
    <>
      <ReportHeading />
      {/* Another report is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <Card>
        <ReportDetailView key={id} id={id} />
      </Card>
    </>
  );
}

function ReportError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_reports" heading={t("client:admin.report.title")} />;
}
