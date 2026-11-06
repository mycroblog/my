// SPDX-License-Identifier: AGPL-3.0-only
// One job's page below its heading (ADR-0004, manage_jobs): its state (when it is due or when it failed, its attempts,
// and "Run again" for a failed job), its other facts (type, creation, worker and lease, keys, the last error), its
// payload, and the records of its runs that did not succeed, read with the page and newest first. Running it again
// makes it a pending job: the page reads it again, the button goes away, and focus moves to the state's heading
// while the result is announced. Types, keys, workers and payloads are identifiers or JSON, read left to right; errors
// are the server's own text.
import {
  Button,
  CodeBlock,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Heading,
  RegionError,
  Spinner,
  Stack,
  Text,
} from "@my/ui";
import { useRef } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { Job, JobAttempt } from "../api/jobs";
import { useJob, useJobAttempts, useRetryJob } from "../hooks/useJobs";
import { DateTime, RequirePermission, Section, useFocusAfterChange } from "../../moderation";
import { RetryJobButton, stateLabels } from "./jobParts";

const outcomeLabels: Readonly<Record<JobAttempt["outcome"], ClientMessageKey>> = {
  retry: "client:admin.jobs.outcome.retry",
  dead: "client:admin.jobs.outcome.dead",
};

export function JobDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_jobs">
      <JobView id={id} />
    </RequirePermission>
  );
}

function JobView({ id }: { id: string }) {
  const t = useT();
  const job = useJob(id);
  return (
    <Stack gap="8">
      <StateSection job={job} />
      <Section surface="card" title={t("client:admin.jobs.detail.details")}>
        <Facts job={job} />
      </Section>
      <Section surface="card" title={t("client:admin.jobs.payload")}>
        <CodeBlock dir="ltr">{job.payload}</CodeBlock>
      </Section>
      <Section surface="card" title={t("client:admin.jobs.history.heading")}>
        <JobAttempts id={job.id} />
      </Section>
    </Stack>
  );
}

function StateSection({ job }: { job: Job }) {
  const t = useT();
  const intl = useIntl();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const focusHeading = useFocusAfterChange(headingRef, job.state);
  const retry = useRetryJob(focusHeading);
  return (
    <Section surface="card" title={t("client:admin.jobs.state")} headingRef={headingRef}>
      <Stack gap="4" align="start">
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
          <DescriptionItem term={t("client:admin.jobs.attempts")}>{intl.formatNumber(job.attempts)}</DescriptionItem>
        </DescriptionList>
        <RetryJobButton job={job} retry={retry} />
      </Stack>
    </Section>
  );
}

function Facts({ job }: { job: Job }) {
  const t = useT();
  return (
    <Stack gap="4">
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.jobs.column.type")}>
          <Text wrap="anywhere" mono dir="ltr">
            {job.type}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.jobs.created_at")}>
          <DateTime value={job.created_at} />
        </DescriptionItem>
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
            <Text wrap="anywhere" mono dir="ltr">
              {job.idempotency_key}
            </Text>
          </DescriptionItem>
        )}
        {job.ordering_key !== undefined && (
          <DescriptionItem term={t("client:admin.jobs.ordering_key")}>
            <Text wrap="anywhere" mono dir="ltr">
              {job.ordering_key}
            </Text>
          </DescriptionItem>
        )}
      </DescriptionList>
      {job.last_error !== undefined && (
        <DescriptionList>
          <DescriptionItem term={t("client:admin.jobs.last_error")}>
            <Text wrap="anywhere" whitespace="preWrap" dir="auto">
              {job.last_error}
            </Text>
          </DescriptionItem>
        </DescriptionList>
      )}
    </Stack>
  );
}

/** The records of the job's runs that did not succeed, newest first, with "Show more" (a short list of evidence, web-client §14). */
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
          headingLevel={3}
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
            headingLevel={3}
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
      <Heading level={3}>
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
