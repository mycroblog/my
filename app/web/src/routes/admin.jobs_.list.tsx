// SPDX-License-Identifier: AGPL-3.0-only
import { Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  type JobFilter,
  jobFilters,
  JobList,
  JobPagesNav,
  jobQueueQueryOptions,
  jobsQueryOptions,
} from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** `state`: the state filter of the job list; omitted, or anything unknown, shows the pending jobs (web-client/README.md S3). */
const searchSchema = z.object({
  state: z.enum(jobFilters).exclude(["pending"]).optional().catch(undefined),
});

/**
 * The job list of the queue dashboard (ADR-0004): the jobs of one state, a page at a time, each leading to its page,
 * and running failed ones again, for manage_jobs. The `_` keeps the page out of the layout of `/admin/jobs`.
 */
export const Route = createFileRoute("/admin/jobs_/list")({
  staticData: { title: "client:admin.jobs.page.list" },
  validateSearch: searchSchema,
  loaderDeps: ({ search: { state } }): { filter: JobFilter } => ({ filter: state ?? "pending" }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps: { filter } }) =>
    loadWithPermission(context, "manage_jobs", () =>
      Promise.all([
        context.queryClient.query({ ...jobQueueQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.infiniteQuery({ ...jobsQueryOptions(context.api, filter), staleTime: Infinity }),
      ]),
    ),
  component: JobListPage,
  errorComponent: JobListError,
});

function JobListPage() {
  const t = useT();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  const changeFilter = (next: JobFilter) => {
    void navigate({ search: next === "pending" ? {} : { state: next } });
  };
  return (
    <>
      <Stack gap="3">
        <PageHeading>{t("client:admin.jobs.title")}</PageHeading>
        <JobPagesNav current="list" />
      </Stack>
      <JobList filter={filter} onFilterChange={changeFilter} />
    </>
  );
}

function JobListError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_jobs" heading={t("client:admin.jobs.title")} />;
}
