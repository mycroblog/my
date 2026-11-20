// SPDX-License-Identifier: AGPL-3.0-only

// The job queue as a holder of `manage_jobs` reads it (ADR-0004, ADR-0058 §1), in three screens. At /admin/jobs, the
// queue's state: how many jobs are in each state, each figure leading to the jobs of that state, when the oldest due
// job has waited since, and a table of the jobs by type and state with how long each type has kept a job waiting; a
// queue that cannot list its pending jobs says so, and leads to the failed ones, which it lists. At /admin/jobs/list,
// the jobs of one state (the pending ones by default) and one type, which the address keeps, a page at a time: each its
// type in the code's face, why it last failed, its state, its attempts, and when it is due or failed. At /admin/jobs/{id}, a job: first, for a failed
// one, Run again, which asks before, since running a job again may repeat what it already did; then its facts, why its
// last run failed and its payload, each to copy, and the runs that did not succeed, the newest first, a table; the jobs
// of its type apart, at the end. A job that is no longer in the queue (it completed, or was deleted) says so.

import {
  jobStates,
  loadAdminJob,
  loadAdminJobs,
  loadAdminQueue,
  refusalDetail,
  useAdminJob,
  useAdminJobAttempts,
  useAdminJobs,
  useAdminQueue,
  useRetryAdminJob,
  type AdminJob,
  type AdminJobQuery,
  type AdminJobState,
  type AdminQueue,
  type AppData,
} from "@my/app-data";
import {
  Pager,
  Box,
  Button,
  CodeText,
  ConfirmDialog,
  FactList,
  FeedNotice,
  FigureTiles,
  FilterBar,
  FilterFacet,
  ModalPresence,
  useAddedFocus,
  RecordTable,
  RelativeTime,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  StateMark,
  TextLink,
  type Fact,
  type FilterValue,
  type RecordColumn,
  type RecordRow,
  type RecordState,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { staffHref, type StaffAddress, type StaffSearch } from "../entries";
import {
  cursorOf,
  filtered,
  ListMore,
  useAddressPages,
  ReadNotice,
  Titled,
  useMoment,
  useSaid,
  useStaffStrings,
  type T,
} from "../parts";
import { CopyValue } from "../settings/parts";
import { CopyAction, SubTitle, useCount } from "./parts";

/** The address of the queue's state. */
const jobsPath = "/admin/jobs";
/** The address of the list of jobs. */
const jobListPath = "/admin/jobs/list";
/** The address of the job of `id`. */
export const jobPath = (id: string): string => `/admin/jobs/${encodeURIComponent(id)}`;

/** How each state of a job looks among the states of the staff's records: a failed job waits for someone. */
const markOf: Readonly<Record<AdminJobState, RecordState>> = {
  queued: "queued",
  running: "running",
  retrying: "retrying",
  dead: "failed",
};

/** The state of a job in words, with its glyph. */
function JobStateMark({ state }: { state: AdminJobState }): ReactNode {
  const { t } = useStaffStrings();
  return <StateMark state={markOf[state]}>{t("staff_operations.jobs.state", { state })}</StateMark>;
}

/** What the list of jobs shows at an address's query: the pending jobs of every type, unless it says otherwise. */
function jobQueryOf(search: StaffSearch): AdminJobQuery {
  const state = jobStates.find((s) => s === search["state"]);
  const type = search["type"];
  return { state, type: type === undefined || type === "" ? undefined : type, cursor: cursorOf(search) };
}

/** The address of the list of jobs at `q`'s first page. */
const listHref = (q: AdminJobQuery): string =>
  staffHref(jobListPath, {
    ...(q.state === undefined ? {} : { state: q.state }),
    ...(q.type === undefined ? {} : { type: q.type }),
  });

/** Reads the queue's state, for the entry's loader. */
export const loadJobQueue = (data: AppData): Promise<void> => loadAdminQueue(data);

/** Reads what the list shows at `search` and the types its filter offers, for the route's loader. */
export async function loadJobList(data: AppData, search: StaffSearch): Promise<void> {
  await Promise.all([loadAdminQueue(data), loadAdminJobs(data, jobQueryOf(search))]);
}

/** Reads the job of `id` and its attempts, for the route's loader. */
export const loadJob = (data: AppData, id: string): Promise<void> => loadAdminJob(data, id);

/** The count of `state`, summed over its types. */
const countOf = (queue: AdminQueue, state: AdminJobState): number =>
  queue.counts.filter((c) => c.state === state).reduce((sum, c) => sum + c.count, 0);

type TypeColumn = AdminJobState | "waiting";

/** The jobs by type, a row a type with its count of each state and how long it has kept a job waiting. */
function TypeTable({ queue }: { queue: AdminQueue }): ReactNode {
  const { t } = useStaffStrings();
  const count = useCount();
  const types = [...new Set(queue.counts.map((c) => c.type))].toSorted();
  const columns: RecordColumn<TypeColumn>[] = [
    ...jobStates.map((state) => ({
      key: state,
      label: t("staff_operations.jobs.state", { state }),
      kind: "figures" as const,
    })),
    { key: "waiting", label: t("staff_operations.queue.column.waiting"), kind: "fit" },
  ];
  const rows = types.map((type): RecordRow<TypeColumn> => {
    const of = (state: AdminJobState) => queue.counts.find((c) => c.type === type && c.state === state)?.count ?? 0;
    const waiting = queue.waits.find((w) => w.type === type)?.oldest_ready_at;
    return {
      id: type,
      head: type,
      code: true,
      href: listHref({ type }),
      cells: {
        ...Object.fromEntries(jobStates.map((state) => [state, count(of(state))])),
        waiting: waiting === undefined ? undefined : <RelativeTime date={waiting} />,
      },
    };
  });
  return (
    <RecordTable
      label={t("staff_operations.queue.types")}
      head={{ label: t("staff_operations.jobs.column.type") }}
      columns={columns}
      rows={rows}
      empty={<FeedNotice start>{t("staff_operations.queue.empty")}</FeedNotice>}
    />
  );
}

/** The queue's state: how many jobs are in each state, how long the oldest due one has waited, and the jobs by type. */
export function JobQueueScreen(): ReactNode {
  const { t } = useStaffStrings();
  const count = useCount();
  const read = useAdminQueue();
  const title = <ScreenTitle glyph="jobs">{t("staff_operations.entry.jobs")}</ScreenTitle>;
  if (read.forbidden) return <FeedNotice lead>{t("staff_operations.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ScreenStack>
        {title}
        <ReadNotice
          status={read.status === "error" ? "error" : "pending"}
          loading={t("staff_operations.queue.loading")}
          failed={t("staff_operations.queue.failed")}
          retry={read.refetch}
        />
      </ScreenStack>
    );
  const queue = read.value;
  const facts: Fact[] = [
    { term: t("staff_operations.queue.adapter"), value: queue.adapter, code: true },
    ...(queue.oldest_ready_at === undefined
      ? []
      : [{ term: t("staff_operations.queue.oldest"), value: <RelativeTime date={queue.oldest_ready_at} /> }]),
  ];
  const failed = countOf(queue, "dead");
  return (
    <ScreenStack>
      {title}
      {/* Failed jobs wait for the reader: said first, with the way to them. */}
      {failed > 0 && (
        <FeedNotice
          alert
          start
          action={<TextLink href={listHref({ state: "dead" })}>{t("staff_operations.queue.to_dead")}</TextLink>}
        >
          {t("staff_operations.queue.failed_count", { count: failed })}
        </FeedNotice>
      )}
      {queue.pending_listing ? (
        <FigureTiles
          label={t("staff_operations.queue.states")}
          items={jobStates.map((state) => ({
            figure: count(countOf(queue, state)),
            label: t("staff_operations.jobs.state", { state }),
            href: listHref({ state }),
          }))}
        />
      ) : (
        <FeedNotice
          start
          action={<TextLink href={listHref({ state: "dead" })}>{t("staff_operations.queue.to_dead")}</TextLink>}
        >
          {t("staff_operations.queue.unlisted")}
        </FeedNotice>
      )}
      <FactList label={t("staff_operations.queue.facts")} items={facts} />
      <Row>
        <TextLink href={jobListPath}>{t("staff_operations.jobs.title")}</TextLink>
      </Row>
      {queue.pending_listing && (
        <Titled title={t("staff_operations.queue.types")}>
          <TypeTable queue={queue} />
        </Titled>
      )}
    </ScreenStack>
  );
}

type JobColumn = "state" | "attempts" | "when";

/** The filters of the list: the state, and the type among those the queue counts. */
function JobFilters({
  q,
  types,
  onChange,
}: {
  q: AdminJobQuery;
  types: readonly string[];
  onChange: (q: AdminJobQuery) => void;
}) {
  const { t } = useStaffStrings();
  const states: FilterValue[] = jobStates.map((state) => ({
    id: state,
    label: t("staff_operations.jobs.state", { state }),
    icon: state === "dead" ? "error" : "jobs",
  }));
  // The type the address names stays offered, though the queue counts none of it now.
  const offered = q.type === undefined || types.includes(q.type) ? types : [...types, q.type];
  return (
    <FilterBar
      label={t("staff_operations.jobs.filters")}
      applied={q.state !== undefined || q.type !== undefined}
      onClear={() => {
        onChange({});
      }}
    >
      <FilterFacet
        label={t("staff_operations.jobs.column.state")}
        any={t("staff_operations.jobs.state.pending")}
        values={states}
        chosen={q.state}
        onChoose={(state) => {
          onChange({ ...q, state: jobStates.find((s) => s === state), cursor: undefined });
        }}
      />
      {offered.length > 0 && (
        <FilterFacet
          label={t("staff_operations.jobs.column.type")}
          any={t("staff_operations.jobs.type.any")}
          values={offered.map((type) => ({ id: type, label: type, icon: "jobs" }))}
          chosen={q.type}
          onChoose={(type) => {
            onChange({ ...q, type, cursor: undefined });
          }}
        />
      )}
    </FilterBar>
  );
}

/** The query of the list's address for `q`, at its first page. */
const searchOf = (q: AdminJobQuery): StaffSearch => ({
  ...(q.state === undefined ? {} : { state: q.state }),
  ...(q.type === undefined ? {} : { type: q.type }),
});

/** The jobs of a state and a type, a page at a time, each leading to its screen. */
export function JobListScreen({ search, onSearch, onBack }: StaffAddress & { onBack: () => void }): ReactNode {
  const pages = useAddressPages({ search, onSearch });
  const { t } = useStaffStrings();
  const moment = useMoment();
  const count = useCount();
  const q = jobQueryOf(search);
  const page = useAdminJobs(q);
  const queue = useAdminQueue().value;
  const types = [...new Set(queue?.counts.map((c) => c.type) ?? [])].toSorted();
  const dead = q.state === "dead";
  const head = (
    <>
      <SubTitle glyph="jobs" back={t("staff_operations.entry.jobs")} onBack={onBack}>
        {t("staff_operations.jobs.title")}
      </SubTitle>
      <JobFilters
        q={q}
        types={types}
        onChange={(next) => {
          onSearch(filtered({}, searchOf(next)));
        }}
      />
    </>
  );
  if (page.forbidden) return <FeedNotice lead>{t("staff_operations.forbidden")}</FeedNotice>;
  if (page.status !== "success")
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={page.status}
          loading={t("staff_operations.jobs.loading")}
          failed={t("staff_operations.jobs.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<JobColumn>[] = [
    { key: "attempts", label: t("staff_operations.jobs.column.attempts"), kind: "figures" },
    {
      key: "when",
      label: t(dead ? "staff_operations.jobs.column.failed_at" : "staff_operations.jobs.column.due"),
      kind: "fit",
      bare: true,
    },
    { key: "state", label: t("staff_operations.jobs.column.state"), kind: "fit", beside: true },
  ];
  const rows = page.items.map((job): RecordRow<JobColumn> => {
    const when = job.state === "dead" ? job.failed_at : job.run_at;
    return {
      id: job.id,
      head: job.type,
      code: true,
      // Why a job that failed did, what the reader opens it for, across the row under its cells. Its ID stands on its
      // screen, with the way to copy it.
      detail:
        job.last_error === undefined || job.last_error === "" ? undefined : (
          <Box as="p" text="caption" tone="muted">
            <CodeText>{firstLine(job.last_error)}</CodeText>
          </Box>
        ),
      href: jobPath(job.id),
      cells: {
        attempts: count(job.attempts),
        // When a job is due or failed, as a time of day: one style for both.
        when: when === undefined ? undefined : moment(when),
        state: <JobStateMark state={job.state} />,
      },
    };
  });
  const unlisted = queue?.pending_listing === false && !dead;
  return (
    <ScreenStack busy={page.stale}>
      {head}
      <RecordTable
        label={t("staff_operations.jobs.table")}
        head={{ label: t("staff_operations.jobs.column.type") }}
        columns={columns}
        rows={rows}
        busy={page.stale}
        empty={
          <FeedNotice start>
            {t(
              unlisted
                ? "staff_operations.queue.unlisted"
                : q.type === undefined && q.state === undefined
                  ? "staff_operations.jobs.empty"
                  : "staff_operations.jobs.empty.filtered",
            )}
          </FeedNotice>
        }
      />
      <Pager {...pages.pager(page.next)} />
    </ScreenStack>
  );
}

/** The title of the list of jobs. */
export function useJobListTitle(): string {
  return useStaffStrings().t("staff_operations.jobs.title");
}

/** The title of the screen of the job of `id`: its type, once read. */
export function useJobTitle(id: string): string | undefined {
  return useAdminJob(id).value?.type;
}

/** The first line of `text`. */
const firstLine = (text: string): string => text.split("\n", 1)[0] ?? text;

/** The payload as its lines are best read: a JSON document laid out, anything else as it is. */
function shownPayload(payload: string): string {
  try {
    return JSON.stringify(JSON.parse(payload), null, 2);
  } catch {
    return payload;
  }
}

/** The facts of a job, its moments in the reader's zone, its ID and keys each with the button that copies it. */
function jobFacts(t: T, moment: (iso: string) => string, count: (n: number) => string, job: AdminJob): Fact[] {
  const at = (term: string, iso: string | undefined): Fact[] =>
    iso === undefined ? [] : [{ term, value: moment(iso) }];
  const code = (term: string, value: string | undefined, copy?: string): Fact[] =>
    value === undefined || value === ""
      ? []
      : [
          {
            term,
            value,
            code: true,
            action: copy === undefined ? undefined : <CopyAction label={copy} value={value} />,
          },
        ];
  return [
    { term: t("staff_operations.jobs.column.state"), value: <JobStateMark state={job.state} /> },
    { term: t("staff_operations.jobs.column.attempts"), value: count(job.attempts) },
    ...at(t("staff_operations.job.created"), job.created_at),
    ...at(t("staff_operations.jobs.column.due"), job.run_at),
    ...at(t("staff_operations.jobs.column.failed_at"), job.failed_at),
    ...code(t("staff_operations.job.worker"), job.worker),
    ...at(t("staff_operations.job.locked_until"), job.locked_until),
    ...code(t("staff_operations.job.id"), job.id, t("staff_operations.copy.id")),
    ...code(t("staff_operations.job.idempotency_key"), job.idempotency_key, t("staff_operations.copy.idempotency_key")),
    ...code(t("staff_operations.job.ordering_key"), job.ordering_key, t("staff_operations.copy.ordering_key")),
  ];
}

type AttemptColumn = "outcome" | "finished" | "worker";

/**
 * The runs of a job that did not succeed, the newest first, a table with Show more while there are older ones: each run
 * its number, whether the job ran again or failed, when, on which worker, and why, in the code's face under it. A job
 * none of whose runs failed has none to show.
 */
function Attempts({ id }: { id: string }): ReactNode {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const list = useAdminJobAttempts(id);
  const added = useAddedFocus(list.items.length);
  const title = t("staff_operations.attempts");
  if (list.status === "success" && list.items.length === 0) return null;
  if (list.status === "pending" || (list.status === "error" && list.items.length === 0))
    return (
      <Titled title={title}>
        <ReadNotice
          status={list.status}
          loading={t("staff_operations.attempts.loading")}
          failed={t("staff_operations.attempts.failed")}
          retry={list.refetch}
        />
      </Titled>
    );
  const columns: RecordColumn<AttemptColumn>[] = [
    { key: "outcome", label: t("staff_operations.attempts.column.outcome"), kind: "fit" },
    { key: "finished", label: t("staff_operations.attempts.column.finished"), kind: "fit" },
    { key: "worker", label: t("staff_operations.job.worker"), kind: "fit" },
  ];
  const rows = list.items.map((a): RecordRow<AttemptColumn> => ({
    id: `${String(a.attempt)}-${a.finished_at}`,
    head: t("staff_operations.attempt.head", { n: a.attempt }),
    cells: {
      outcome: (
        <StateMark state={a.outcome === "dead" ? "failed" : "retrying"}>
          {t("staff_operations.attempt.outcome", { outcome: a.outcome })}
        </StateMark>
      ),
      finished: moment(a.finished_at),
      worker: <CodeText>{a.worker}</CodeText>,
    },
    detail: (
      <Box as="p" text="caption">
        <CodeText>{a.error}</CodeText>
      </Box>
    ),
  }));
  return (
    <Titled title={title}>
      <Stack gap="entry-room">
        <RecordTable
          label={title}
          head={{ label: t("staff_operations.attempts.column.attempt") }}
          columns={columns}
          rows={rows}
          itemRef={added.item}
        />
        <ListMore list={list} added={added} />
      </Stack>
    </Titled>
  );
}

/** A job: Run again for a failed one, its facts, its last error and payload, its attempts, and its type's jobs. */
function JobView({ job, onBack }: { job: AdminJob; onBack: () => void }): ReactNode {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const count = useCount();
  const retry = useRetryAdminJob();
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const { say, status } = useSaid();
  const run = () => {
    setBusy(true);
    retry(job.id).then(
      () => {
        setBusy(false);
        setAsking(false);
        say(t("staff_operations.job.retried"));
      },
      (error: unknown) => {
        setBusy(false);
        setAsking(false);
        say(refusalDetail(error) ?? t("staff_operations.job.retry_failed"), true);
      },
    );
  };
  return (
    <>
      <ScreenStack>
        <SubTitle glyph="jobs" back={t("staff_operations.jobs.title")} onBack={onBack}>
          {job.type}
        </SubTitle>
        {/* What the reader does with the job first, on a row of its own under the title. */}
        <Row wrap align="center" gap="screen-tab-gap">
          {job.state === "dead" && (
            <Button
              kind="primary"
              size="timeline"
              busy={busy}
              onPress={() => {
                setAsking(true);
              }}
            >
              {t("staff_operations.job.retry")}
            </Button>
          )}
          {status}
        </Row>
        <FactList label={t("staff_operations.job.facts")} items={jobFacts(t, moment, count, job)} />
        {job.last_error !== undefined && job.last_error !== "" && (
          <Titled title={t("staff_operations.job.last_error")}>
            <CopyValue
              label={t("staff_operations.job.last_error")}
              value={job.last_error}
              copy={t("staff_operations.copy.reason")}
              code
            />
          </Titled>
        )}
        <Titled title={t("staff_operations.job.payload")}>
          <CopyValue
            label={t("staff_operations.job.payload")}
            value={shownPayload(job.payload)}
            copy={t("staff_operations.copy.payload")}
            code
          />
        </Titled>
        <Attempts id={job.id} />
        <Row>
          <TextLink href={listHref({ state: job.state === "dead" ? "dead" : undefined, type: job.type })}>
            {t("staff_operations.job.of_type", { type: job.type })}
          </TextLink>
        </Row>
      </ScreenStack>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            key="retry"
            title={t("staff_operations.job.retry.title")}
            cancel={t("staff_operations.cancel")}
            confirm={t("staff_operations.job.retry")}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={run}
          >
            {t("staff_operations.job.retry.text", { type: job.type })}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}

/** The job of `id`, or why it shows none. */
export function JobScreen({ id, onBack }: { id: string; onBack: () => void }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminJob(id);
  if (read.missing)
    return (
      <FeedNotice alert lead action={<TextLink href={jobListPath}>{t("staff_operations.jobs.title")}</TextLink>}>
        {t("staff_operations.job.missing")}
      </FeedNotice>
    );
  if (read.forbidden) return <FeedNotice lead>{t("staff_operations.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_operations.job.loading")}
        failed={t("staff_operations.job.failed")}
        retry={read.refetch}
      />
    );
  return <JobView job={read.value} onBack={onBack} />;
}

/** Where the list's way back leads: the queue's state. */
export const jobQueuePath = jobsPath;
/** Where a job's way back leads: the list. */
export const jobsListPath = jobListPath;
