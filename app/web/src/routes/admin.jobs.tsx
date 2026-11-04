// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { type JobFilter, jobFilters, JobDashboard, jobQueueQueryOptions, jobsQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** `state`: the state filter of the job list; omitted, or anything unknown, shows the pending jobs (web-client/README.md S3). */
const searchSchema = z.object({
  state: z.enum(jobFilters).exclude(["pending"]).optional().catch(undefined),
});

/** The queue dashboard (ADR-0004): the job queue's state, its jobs by state with their attempts, and running failed ones again, for manage_jobs. */
export const Route = createFileRoute("/admin/jobs")({
  staticData: { title: "client:admin.jobs.title" },
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
  component: JobsPage,
  errorComponent: JobsError,
});

function JobsPage() {
  const t = useT();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  const changeFilter = (next: JobFilter) => {
    void navigate({ search: next === "pending" ? {} : { state: next } });
  };
  return (
    <>
      <PageHeading>{t("client:admin.jobs.title")}</PageHeading>
      <JobDashboard filter={filter} onFilterChange={changeFilter} />
    </>
  );
}

function JobsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_jobs" heading={t("client:admin.jobs.title")} />;
}
