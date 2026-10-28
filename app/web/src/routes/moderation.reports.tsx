// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  reportAssigneeFilters,
  reportCategories,
  type ReportListQuery,
  ReportQueue,
  reportsQueryOptions,
} from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The filters of the queue (web-client/README.md S3). `state`: the tab, omitted for the open reports; `category`: a
 * report category; `assignee`: `me` or `none`; `order`: `oldest`, omitted for the newest first. Anything unknown is
 * dropped.
 */
const searchSchema = z.object({
  state: z.enum(["resolved"]).optional().catch(undefined),
  category: z.enum(reportCategories).optional().catch(undefined),
  assignee: z.enum(reportAssigneeFilters).optional().catch(undefined),
  order: z.enum(["oldest"]).optional().catch(undefined),
});

type Search = z.infer<typeof searchSchema>;

/** The search of the queue under `list`; the defaults are left out. */
function searchOf(list: ReportListQuery): Search {
  return {
    state: list.state === "resolved" ? list.state : undefined,
    category: list.category,
    assignee: list.assignee,
    order: list.order === "oldest" ? list.order : undefined,
  };
}

/** The reports to the moderators, open or resolved, filtered and in an order (FR-701, FR-710), for manage_reports. */
export const Route = createFileRoute("/moderation/reports")({
  staticData: { title: "client:admin.reports.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): { list: ReportListQuery } => ({
    list: {
      state: search.state ?? "open",
      category: search.category,
      assignee: search.assignee,
      order: search.order ?? "newest",
    },
  }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps: { list } }) =>
    loadWithPermission(context, "manage_reports", () =>
      context.queryClient.infiniteQuery({ ...reportsQueryOptions(context.api, list), staleTime: Infinity }),
    ),
  component: ReportsPage,
  errorComponent: ReportsError,
});

function ReportsPage() {
  const t = useT();
  const { list } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.reports.title")}</PageHeading>
      <ReportQueue
        list={list}
        onListChange={(next) => {
          void navigate({ search: searchOf(next) });
        }}
      />
    </>
  );
}

function ReportsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_reports" heading={t("client:admin.reports.title")} />;
}
