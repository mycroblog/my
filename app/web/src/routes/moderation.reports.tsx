// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { type ReportFilter, ReportQueue, reportsQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** `state`: the tab of the queue; omitted, or anything unknown, shows the open reports (web-client/README.md S3). */
const searchSchema = z.object({
  state: z.enum(["resolved"]).optional().catch(undefined),
});

/** The reports to the moderators, open or resolved, newest first (FR-701, FR-710), for manage_reports. */
export const Route = createFileRoute("/moderation/reports")({
  staticData: { title: "client:admin.reports.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search: { state } }): { filter: ReportFilter } => ({ filter: state ?? "open" }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps: { filter } }) =>
    loadWithPermission(context, "manage_reports", () =>
      context.queryClient.infiniteQuery({ ...reportsQueryOptions(context.api, filter), staleTime: Infinity }),
    ),
  component: ReportsPage,
  errorComponent: ReportsError,
});

function ReportsPage() {
  const t = useT();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.reports.title")}</PageHeading>
      <ReportQueue
        filter={filter}
        onFilterChange={(next) => {
          void navigate({ search: { state: next === "open" ? undefined : next } });
        }}
      />
    </>
  );
}

function ReportsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_reports" heading={t("client:admin.reports.title")} />;
}
