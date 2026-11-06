// SPDX-License-Identifier: AGPL-3.0-only
// The job list of the queue dashboard below its heading and local navigation (ADR-0004): a toolbar with the state
// filter (kept in the URL by the route, web-client/README.md S3), then the jobs of that state in the admin mode's
// table form, one page at a time with previous / next (web-client §14 一覧の表). A row has the job's type (a link to
// its page, which has every other fact, the error, the payload and the attempts), its state, when it is due or when it
// failed, and, for a failed job, "Run again". A job run again leaves the failed jobs, and focus moves to the next row's
// button (lib/useFocusAfterRemoval.ts). A queue that cannot list its pending jobs (Cloud Tasks) says so instead of
// their list, and still lists the failed jobs.
import { EmptyState, Link, ListToolbar, Select, SelectItem, Stack } from "@my/ui";
import { useRef } from "react";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { type Job, type JobFilter, jobFilters, jobPath } from "../api/jobs";
import { useJobQueue, useJobs, useRetryJob } from "../hooks/useJobs";
import { DateTime, type RecordColumns, RecordTable, RequirePermission } from "../../moderation";
import { filterLabels, RetryJobButton, stateLabels } from "./jobParts";

function isFilter(key: unknown): key is JobFilter {
  return jobFilters.some((filter) => filter === key);
}

type JobListProps = {
  /** The selected filter, from the URL. */
  filter: JobFilter;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: JobFilter) => void;
};

export function JobList(props: JobListProps) {
  return (
    <RequirePermission permission="manage_jobs">
      <JobListView {...props} />
    </RequirePermission>
  );
}

function JobListView({ filter, onFilterChange }: JobListProps) {
  const t = useT();
  const queue = useJobQueue();
  return (
    <Stack gap="4">
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.jobs.filter.label")}
          value={filter}
          onChange={(key) => {
            if (isFilter(key)) onFilterChange(key);
          }}
        >
          {jobFilters.map((item) => (
            <SelectItem key={item} id={item}>
              {t(filterLabels[item])}
            </SelectItem>
          ))}
        </Select>
      </ListToolbar>
      {filter !== "dead" && !queue.pending_listing ? (
        <EmptyState
          headingLevel={2}
          heading={t("client:admin.jobs.unlisted")}
          description={t("client:admin.jobs.unlisted.description")}
        />
      ) : (
        // Another filter is another list: it starts at its first page.
        <Jobs key={filter} filter={filter} />
      )}
    </Stack>
  );
}

function Jobs({ filter }: { filter: JobFilter }) {
  const t = useT();
  const listRef = useRef<HTMLDivElement>(null);
  const list = useJobs(filter);
  const removed = useFocusAfterRemoval(
    listRef,
    list.items.map((job) => job.id),
  );
  const retry = useRetryJob(removed);
  const dead = filter === "dead";
  const columns: RecordColumns<Job> = [
    {
      id: "type",
      header: t("client:admin.jobs.column.type"),
      wrap: "anywhere",
      // Job types are identifiers, read left to right in every locale.
      cell: (job) => (
        <Link href={jobPath(job.id)} variant="standalone" wrap="anywhere">
          <span dir="ltr">{job.type}</span>
        </Link>
      ),
    },
    { id: "state", header: t("client:admin.jobs.state"), cell: (job) => t(stateLabels[job.state]) },
    {
      id: "date",
      header: t(dead ? "client:admin.jobs.failed_at" : "client:admin.jobs.run_at"),
      cell: (job) => {
        const at = dead ? job.failed_at : job.run_at;
        return at === undefined ? null : <DateTime value={at} />;
      },
    },
    ...(dead
      ? [
          {
            id: "actions",
            kind: "actions" as const,
            header: t("client:admin.list.column.actions"),
            cell: (job: Job) => <RetryJobButton job={job} retry={retry} />,
          },
        ]
      : []),
  ];
  return (
    <RecordTable
      list={list}
      caption={t("client:admin.jobs.list.caption", { filter: t(filterLabels[filter]) })}
      columns={columns}
      getId={(job) => job.id}
      headingLevel={2}
      listRef={listRef}
      empty={
        <EmptyState headingLevel={2} heading={t(dead ? "client:admin.jobs.empty.dead" : "client:admin.jobs.empty")} />
      }
    />
  );
}
