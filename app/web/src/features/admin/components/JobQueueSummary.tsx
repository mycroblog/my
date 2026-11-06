// SPDX-License-Identifier: AGPL-3.0-only
// The first page of the queue dashboard below its heading and local navigation (ADR-0004): the state of the job
// queue. How long its oldest due job waits and its jobs of each state are tiles (ADR-0110), each state a link to its
// jobs on the list page, the failed ones marked when there are any; then the queue's kind and its jobs by type with
// how long each type keeps a job waiting. A queue that cannot list its pending jobs (Cloud Tasks) says so, and shows
// only the failed jobs.
import { Card, DescriptionItem, DescriptionList, Heading, Hint, Stack, StatTile, StatTileGrid } from "@my/ui";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { type JobQueue, jobListPath, type JobState, jobStates } from "../api/jobs";
import { useJobWait } from "../hooks/useJobWait";
import { useJobQueue } from "../hooks/useJobs";
import { RequirePermission, Section } from "../../moderation";
import { stateLabels } from "./jobParts";

export function JobQueueSummary() {
  return (
    <RequirePermission permission="manage_jobs">
      <QueueSection />
    </RequirePermission>
  );
}

/** The number of jobs of `state` in `queue`, of every type or of `type`. */
function stateCount(queue: JobQueue, state: JobState, type?: string): number {
  return queue.counts
    .filter((item) => item.state === state && (type === undefined || item.type === type))
    .reduce((sum, item) => sum + item.count, 0);
}

function QueueSection() {
  const t = useT();
  const queue = useJobQueue();
  return (
    <Section surface="card" title={t("client:admin.jobs.queue.heading")}>
      <Card>
        <QueueState queue={queue} />
      </Card>
    </Section>
  );
}

function QueueState({ queue }: { queue: JobQueue }) {
  const t = useT();
  const intl = useIntl();
  const wait = useJobWait(queue.oldest_ready_at);
  const types = [...new Set(queue.counts.map((count) => count.type))];
  const dead = stateCount(queue, "dead");
  const shownStates = queue.pending_listing ? jobStates : (["dead"] as const);
  return (
    <Stack gap="4">
      <StatTileGrid aria-label={t("client:admin.jobs.queue.by_state")}>
        {queue.pending_listing && <StatTile as="li" label={t("client:admin.overview.health.job_wait")} {...wait} />}
        {shownStates.map((state) => (
          <StatTile
            key={state}
            as="li"
            label={t(stateLabels[state])}
            value={intl.formatNumber(stateCount(queue, state))}
            href={jobListPath(state)}
            {...(state === "dead" &&
              dead > 0 && { status: "danger", statusLabel: t("client:admin.overview.health.failed_jobs.status") })}
          />
        ))}
      </StatTileGrid>
      {!queue.pending_listing && <Hint>{t("client:admin.jobs.queue.no_listing")}</Hint>}
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.jobs.queue.adapter")}>
          <code dir="ltr">{queue.adapter}</code>
        </DescriptionItem>
      </DescriptionList>
      {queue.pending_listing && types.length > 0 && (
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
                    {intl.formatNumber(stateCount(queue, state, type))}
                  </DescriptionItem>
                ))}
                <TypeWait oldestReadyAt={queue.waits.find((wait) => wait.type === type)?.oldest_ready_at} />
              </DescriptionList>
            </Stack>
          ))}
        </Stack>
      )}
    </Stack>
  );
}

/** How long a type keeps a job waiting, which shows a type that no worker runs (ADR-0004 §17). */
function TypeWait({ oldestReadyAt }: { oldestReadyAt: string | undefined }) {
  const t = useT();
  const wait = useJobWait(oldestReadyAt);
  return <DescriptionItem term={t("client:admin.overview.health.job_wait")}>{wait.value}</DescriptionItem>;
}
