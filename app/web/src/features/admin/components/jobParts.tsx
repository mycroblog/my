// SPDX-License-Identifier: AGPL-3.0-only
// What the pages of the queue dashboard share (ADR-0004): the names of the states and filters, the local navigation
// between the queue's state and the job list (web-client §14 画面の分け方), and the button that runs a failed job again.
import { Button, LocalNav } from "@my/ui";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { Job, JobFilter, JobState } from "../api/jobs";
import type { RetryJob } from "../hooks/useJobs";

export const filterLabels: Readonly<Record<JobFilter, ClientMessageKey>> = {
  pending: "client:admin.jobs.filter.pending",
  queued: "client:admin.jobs.state.queued",
  running: "client:admin.jobs.state.running",
  retrying: "client:admin.jobs.state.retrying",
  dead: "client:admin.jobs.state.dead",
};

export const stateLabels: Readonly<Record<JobState, ClientMessageKey>> = {
  queued: "client:admin.jobs.state.queued",
  running: "client:admin.jobs.state.running",
  retrying: "client:admin.jobs.state.retrying",
  dead: "client:admin.jobs.state.dead",
};

/** The pages of the screen under its heading: the queue's state (`/admin/jobs`) and the job list (`/admin/jobs/list`). */
export function JobPagesNav({ current }: { current: "queue" | "list" }) {
  const t = useT();
  return (
    <LocalNav
      label={t("client:admin.jobs.pages")}
      items={[
        { href: "/admin/jobs", label: t("client:admin.jobs.page.queue"), current: current === "queue" },
        { href: "/admin/jobs/list", label: t("client:admin.jobs.page.list"), current: current === "list" },
      ]}
    />
  );
}

/**
 * "Run again" of a failed job; nothing for any other job. Its accessible name has the job's type and when it failed,
 * as a list has a button per row.
 */
export function RetryJobButton({ job, retry }: { job: Job; retry: RetryJob }) {
  const t = useT();
  const intl = useIntl();
  if (job.state !== "dead" || job.failed_at === undefined) return null;
  return (
    <Button
      size="sm"
      isPending={retry.pendingIds.includes(job.id)}
      aria-label={t("client:admin.jobs.retry_job", {
        type: job.type,
        date: intl.formatDate(job.failed_at, { dateStyle: "medium", timeStyle: "short" }),
      })}
      onPress={() => {
        retry.retry(job);
      }}
    >
      {t("client:admin.jobs.run_again")}
    </Button>
  );
}
