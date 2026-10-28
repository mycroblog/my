// SPDX-License-Identifier: AGPL-3.0-only
// The queue dashboard below its heading (ADR-0064): the state of the job queue (its jobs by state and type, and since
// when the oldest due job waits), then the jobs under tabs that filter by state, kept in the URL by the route
// (web-client/README.md S3). Each job shows its state, why its last run failed, its keys and payload, and a disclosure with
// the records of its runs that did not succeed; a dead job also has a button that runs it again, after which it leaves
// the list and focus moves to the next job's button. A queue that cannot list its pending jobs (Cloud Tasks, SQS)
// says so, and still lists the dead jobs. The tabs follow the WAI-ARIA APG tabs pattern with automatic activation
// (React Aria Tabs); only the selected panel is mounted.
import {
  Box,
  Button,
  Card,
  CodeBlock,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Heading,
  Hint,
  RegionError,
  SectionHeader,
  Spinner,
  Stack,
  Tab,
  TabList,
  TabPanel,
  Tabs,
  Text,
} from "@my/ui";
import { useId, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import {
  type Job,
  type JobAttempt,
  type JobFilter,
  jobFilters,
  type JobQueue,
  type JobState,
  jobStates,
} from "../api/jobs";
import { type RetryJob, useJobAttempts, useJobQueue, useJobs, useRetryJob } from "../hooks/useJobs";
import { DateTime, Section, RecordList, RequirePermission } from "../../moderation";

const filterLabels: Readonly<Record<JobFilter, ClientMessageKey>> = {
  pending: "client:admin.jobs.filter.pending",
  queued: "client:admin.jobs.state.queued",
  running: "client:admin.jobs.state.running",
  retrying: "client:admin.jobs.state.retrying",
  dead: "client:admin.jobs.state.dead",
};

const stateLabels: Readonly<Record<JobState, ClientMessageKey>> = {
  queued: "client:admin.jobs.state.queued",
  running: "client:admin.jobs.state.running",
  retrying: "client:admin.jobs.state.retrying",
  dead: "client:admin.jobs.state.dead",
};

const outcomeLabels: Readonly<Record<JobAttempt["outcome"], ClientMessageKey>> = {
  retry: "client:admin.jobs.outcome.retry",
  dead: "client:admin.jobs.outcome.dead",
};

function isFilter(key: unknown): key is JobFilter {
  return jobFilters.some((filter) => filter === key);
}

export type JobDashboardProps = {
  /** The selected filter, from the URL. */
  filter: JobFilter;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: JobFilter) => void;
};

export function JobDashboard(props: JobDashboardProps) {
  return (
    <RequirePermission permission="manage_jobs">
      <Dashboard {...props} />
    </RequirePermission>
  );
}

function Dashboard({ filter, onFilterChange }: JobDashboardProps) {
  const t = useT();
  const queue = useJobQueue();
  return (
    <Stack gap="8">
      <Section surface="card" title={t("client:admin.jobs.queue.heading")}>
        <Card>
          <QueueSummary queue={queue} />
        </Card>
      </Section>
      <Section surface="card" title={t("client:admin.jobs.list.heading")}>
        <Tabs
          selectedKey={filter}
          onSelectionChange={(key) => {
            if (isFilter(key)) onFilterChange(key);
          }}
        >
          <TabList aria-label={t("client:admin.jobs.filter.label")}>
            {jobFilters.map((item) => (
              <Tab key={item} id={item}>
                {t(filterLabels[item])}
              </Tab>
            ))}
          </TabList>
          {/* One panel per tab: React Aria mounts only the selected one, and each keeps the id its tab's aria-controls names. */}
          {jobFilters.map((item) => (
            <TabPanel key={item} id={item}>
              {filter !== "dead" && !queue.pending_listing ? (
                <EmptyState
                  headingLevel={3}
                  heading={t("client:admin.jobs.unlisted")}
                  description={t("client:admin.jobs.unlisted.description")}
                />
              ) : (
                <Card>
                  <Jobs filter={filter} />
                </Card>
              )}
            </TabPanel>
          ))}
        </Tabs>
      </Section>
    </Stack>
  );
}

/** The number of jobs in each state, overall and by type, and since when the oldest due job waits. */
function QueueSummary({ queue }: { queue: JobQueue }) {
  const t = useT();
  const intl = useIntl();
  const types = [...new Set(queue.counts.map((count) => count.type))];
  const count = (state: JobState, type?: string) =>
    queue.counts
      .filter((item) => item.state === state && (type === undefined || item.type === type))
      .reduce((sum, item) => sum + item.count, 0);
  return (
    <Stack gap="4">
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.jobs.queue.adapter")}>
          <code dir="ltr">{queue.adapter}</code>
        </DescriptionItem>
        {queue.pending_listing && (
          <DescriptionItem term={t("client:admin.jobs.queue.oldest_ready")}>
            {queue.oldest_ready_at === undefined ? (
              t("client:admin.jobs.queue.none_waiting")
            ) : (
              <DateTime value={queue.oldest_ready_at} />
            )}
          </DescriptionItem>
        )}
      </DescriptionList>
      {queue.pending_listing ? (
        <>
          <DescriptionList columns={4}>
            {jobStates.map((state) => (
              <DescriptionItem key={state} term={t(stateLabels[state])}>
                {intl.formatNumber(count(state))}
              </DescriptionItem>
            ))}
          </DescriptionList>
          {types.length > 0 && (
            <Stack as="ul" divided aria-label={t("client:admin.jobs.queue.by_type")}>
              {types.map((type) => (
                <Stack as="li" container gap="2" paddingY="3" key={type}>
                  {/* Job types are identifiers, read left to right in every locale. */}
                  <Heading level={3} wrap="anywhere" mono>
                    <span dir="ltr">{type}</span>
                  </Heading>
                  <DescriptionList columns={4}>
                    {jobStates.map((state) => (
                      <DescriptionItem key={state} term={t(stateLabels[state])}>
                        {intl.formatNumber(count(state, type))}
                      </DescriptionItem>
                    ))}
                  </DescriptionList>
                </Stack>
              ))}
            </Stack>
          )}
        </>
      ) : (
        <Hint>{t("client:admin.jobs.queue.no_listing")}</Hint>
      )}
    </Stack>
  );
}

function Jobs({ filter }: { filter: JobFilter }) {
  const t = useT();
  const listRef = useRef<HTMLUListElement>(null);
  const list = useJobs(filter);
  const removed = useFocusAfterRemoval(
    listRef,
    list.items.map((job) => job.id),
  );
  const retry = useRetryJob(removed);
  return (
    <RecordList
      list={list}
      getId={(job) => job.id}
      listRef={listRef}
      empty={
        <EmptyState
          headingLevel={3}
          heading={t(filter === "dead" ? "client:admin.jobs.empty.dead" : "client:admin.jobs.empty")}
        />
      }
    >
      {(job) => <JobRow job={job} retry={retry} />}
    </RecordList>
  );
}

function JobRow({ job, retry }: { job: Job; retry: RetryJob }) {
  const t = useT();
  const intl = useIntl();
  return (
    <Stack gap="3">
      <SectionHeader
        actions={
          job.state === "dead" &&
          job.failed_at !== undefined && (
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
              {t("client:admin.jobs.retry")}
            </Button>
          )
        }
      >
        {/* Job types are identifiers, read left to right in every locale. */}
        <Heading level={3} wrap="anywhere" mono>
          <span dir="ltr">{job.type}</span>
        </Heading>
      </SectionHeader>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.jobs.state")}>{t(stateLabels[job.state])}</DescriptionItem>
        {job.failed_at !== undefined && (
          <DescriptionItem term={t("client:admin.jobs.failed_at")}>
            <DateTime value={job.failed_at} />
          </DescriptionItem>
        )}
        {job.run_at !== undefined && (
          <DescriptionItem term={t("client:admin.jobs.run_at")}>
            <DateTime value={job.run_at} />
          </DescriptionItem>
        )}
        <DescriptionItem term={t("client:admin.jobs.created_at")}>
          <DateTime value={job.created_at} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.jobs.attempts")}>{intl.formatNumber(job.attempts)}</DescriptionItem>
        {job.worker !== undefined && (
          <DescriptionItem term={t("client:admin.jobs.worker")}>
            <code dir="ltr">{job.worker}</code>
          </DescriptionItem>
        )}
        {job.locked_until !== undefined && (
          <DescriptionItem term={t("client:admin.jobs.locked_until")}>
            <DateTime value={job.locked_until} />
          </DescriptionItem>
        )}
        {job.idempotency_key !== undefined && (
          <DescriptionItem term={t("client:admin.jobs.idempotency_key")}>
            <code dir="ltr">{job.idempotency_key}</code>
          </DescriptionItem>
        )}
        {job.ordering_key !== undefined && (
          <DescriptionItem term={t("client:admin.jobs.ordering_key")}>
            <code dir="ltr">{job.ordering_key}</code>
          </DescriptionItem>
        )}
      </DescriptionList>
      <DescriptionList>
        {job.last_error !== undefined && (
          <DescriptionItem term={t("client:admin.jobs.last_error")}>
            {/* Errors and payloads are the server's own text, in English or JSON. */}
            <Text whitespace="preWrap" dir="auto">
              {job.last_error}
            </Text>
          </DescriptionItem>
        )}
        <DescriptionItem term={t("client:admin.jobs.payload")}>
          <CodeBlock dir="ltr">{job.payload}</CodeBlock>
        </DescriptionItem>
      </DescriptionList>
      <JobHistory job={job} />
    </Stack>
  );
}

/**
 * The records of the job's runs that did not succeed, as a WAI-ARIA disclosure (web-client/README.md Y8): a button with
 * aria-expanded and aria-controls, and a panel that is `hidden` while closed. The records are read when it opens.
 */
function JobHistory({ job }: { job: Job }) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);
  const panelId = useId();
  return (
    <Stack gap="2" align="start">
      <Button
        size="sm"
        aria-expanded={expanded}
        aria-controls={panelId}
        aria-label={t("client:admin.jobs.history.toggle_job", { type: job.type })}
        onPress={() => {
          setExpanded(!expanded);
        }}
      >
        {t("client:admin.jobs.history.toggle")}
      </Button>
      <Box width="full" id={panelId} hidden={!expanded}>
        {expanded && <JobAttempts id={job.id} />}
      </Box>
    </Stack>
  );
}

function JobAttempts({ id }: { id: string }) {
  const t = useT();
  const attempts = useJobAttempts(id);
  switch (attempts.status) {
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner label={t("client:admin.jobs.history.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError
          heading={t("client:admin.jobs.history.failed")}
          headingLevel={4}
          reason={attempts.reason}
          retryLabel={t("client:admin.jobs.history.retry")}
          onRetry={attempts.retry}
          isRetrying={attempts.isRetrying}
        />
      );
    case "ready":
      if (attempts.items.length === 0) {
        return (
          <EmptyState
            headingLevel={4}
            heading={t("client:admin.jobs.history.empty")}
            description={t("client:admin.jobs.history.empty.description")}
          />
        );
      }
      return (
        <Stack gap="3">
          <Stack as="ol" divided border="md">
            {attempts.items.map((attempt) => (
              <Stack
                as="li"
                container
                gap="2"
                paddingX="3"
                paddingY="2"
                key={`${attempt.finished_at} ${String(attempt.attempt)}`}
              >
                <AttemptRecord attempt={attempt} />
              </Stack>
            ))}
          </Stack>
          {attempts.moreError !== undefined && (
            <RegionError heading={t("client:admin.jobs.history.more_failed")} reason={attempts.moreError} />
          )}
          {attempts.hasMore && (
            <div>
              <Button size="sm" onPress={attempts.loadMore} isPending={attempts.isLoadingMore}>
                {t("client:admin.list.more")}
              </Button>
            </div>
          )}
        </Stack>
      );
  }
}

function AttemptRecord({ attempt }: { attempt: JobAttempt }) {
  const t = useT();
  return (
    <>
      <Heading level={4}>
        {t("client:admin.jobs.history.attempt", {
          number: attempt.attempt,
          outcome: t(outcomeLabels[attempt.outcome]),
        })}
      </Heading>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.jobs.history.started_at")}>
          <DateTime value={attempt.started_at} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.jobs.history.finished_at")}>
          <DateTime value={attempt.finished_at} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.jobs.worker")}>
          <code dir="ltr">{attempt.worker}</code>
        </DescriptionItem>
      </DescriptionList>
      <DescriptionList>
        <DescriptionItem term={t("client:admin.jobs.history.error")}>
          <Text wrap="anywhere" whitespace="preWrap" dir="auto">
            {attempt.error}
          </Text>
        </DescriptionItem>
      </DescriptionList>
    </>
  );
}
