// SPDX-License-Identifier: AGPL-3.0-only
import { Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { JobPagesNav, JobQueueSummary, jobQueueQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The first page of the queue dashboard (ADR-0004): the state of the job queue, its jobs by state and type and how
 * long they wait, for manage_jobs. The job list is the next page (`/admin/jobs/list`).
 */
export const Route = createFileRoute("/admin/jobs")({
  staticData: { title: "client:admin.jobs.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_jobs", () =>
      context.queryClient.query({ ...jobQueueQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: JobQueuePage,
  errorComponent: JobsError,
});

function JobQueuePage() {
  const t = useT();
  return (
    <>
      <Stack gap="3">
        <PageHeading>{t("client:admin.jobs.title")}</PageHeading>
        <JobPagesNav current="queue" />
      </Stack>
      <JobQueueSummary />
    </>
  );
}

function JobsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_jobs" heading={t("client:admin.jobs.title")} />;
}
