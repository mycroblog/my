// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { JobDetail, jobQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the layout of `/admin/jobs`; `/admin/jobs/list` is a page of its own. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * One job of the queue (ADR-0004): its state with running a failed one again, its facts, its payload and the records
 * of its runs that did not succeed, for manage_jobs. A completed job is no longer there: the page is not found.
 */
export const Route = createFileRoute("/admin/jobs_/$id")({
  staticData: { title: "client:admin.jobs.detail_title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_jobs", () =>
      context.queryClient.query({ ...jobQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: JobPage,
  errorComponent: JobError,
});

function JobHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.jobs.detail_title")}</PageHeading>
      <Link href="/admin/jobs/list" variant="standalone" size="sm">
        {t("client:admin.jobs.back")}
      </Link>
    </Stack>
  );
}

function JobPage() {
  const { id } = Route.useParams();
  return (
    <>
      <JobHeading />
      {/* Another job is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <JobDetail key={id} id={id} />
    </>
  );
}

function JobError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_jobs" heading={t("client:admin.jobs.detail_title")} />;
}
