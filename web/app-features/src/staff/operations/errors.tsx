// SPDX-License-Identifier: AGPL-3.0-only

// The errors the server and the web client reported, as a holder of `manage_errors` reads them (ADR-0103 §8), in two
// screens. At /admin/errors, the groups, the open ones by default, filtered by state and source, which the address
// keeps, the most recently seen first, a page at a time: each the first line of its message and where it happened, in
// the code's face, its state, its occurrences of the last 24 hours and 7 days, and when it was last seen. At
// /admin/errors/{id}, a group: first what the reader does with it (resolve or mute an open one, mute a resolved one,
// unmute a muted one), what came of it said in the screen's status; then its facts; its occurrences of the last 24
// hours and 7 days, a table; its latest occurrence, with the request ID and the trace ID to search the logs by, each
// with its copy button, and its stack to copy; its occurrences by hour, the newest first, a table of figures; and the
// changes of its state, the newest first.

import {
  errorGroupSources,
  errorGroupStates,
  loadErrorGroup,
  loadErrorGroups,
  refusalDetail,
  useErrorGroup,
  useErrorGroups,
  useErrorGroupWrites,
  type AppData,
  type ErrorGroup,
  type ErrorGroupDetail,
  type ErrorGroupEvent,
  type ErrorGroupQuery,
  type ErrorGroupState,
} from "@my/app-data";
import {
  Box,
  Button,
  CodeText,
  FactList,
  FeedNotice,
  FilterBar,
  FilterFacet,
  FinanceRow,
  FinanceTable,
  LogEntry,
  LogList,
  MoreRecords,
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
import { cursorOf, filtered, Pages, ReadNotice, Titled, useMoment, useSaid, useStaffStrings, type T } from "../parts";
import { CopyValue } from "../settings/parts";
import { CopyAction, SubTitle, useCount } from "./parts";

/** The address of the error groups. */
const errorsPath = "/admin/errors";
/** The address of the group of `id`. */
export const errorGroupPath = (id: string): string => `${errorsPath}/${encodeURIComponent(id)}`;

/** How each state of a group looks among the states of the staff's records. */
const markOf: Readonly<Record<ErrorGroupState, RecordState>> = { open: "open", resolved: "resolved", muted: "muted" };

/** The state of a group in words, with its glyph. */
function GroupStateMark({ state }: { state: ErrorGroupState }): ReactNode {
  const { t } = useStaffStrings();
  return <StateMark state={markOf[state]}>{t("staff_operations.error_groups.state", { state })}</StateMark>;
}

/** What the list shows at an address's query: the open groups of both sources, unless it says otherwise. */
function errorQueryOf(search: StaffSearch): ErrorGroupQuery {
  const { state, source } = search;
  return {
    state: state === "any" ? undefined : (errorGroupStates.find((s) => s === state) ?? "open"),
    source: errorGroupSources.find((s) => s === source),
    cursor: cursorOf(search),
  };
}

/** The query of the list's address for `q`, at its first page. */
const searchOf = (q: ErrorGroupQuery): StaffSearch => ({
  ...(q.state === "open" ? {} : { state: q.state ?? "any" }),
  ...(q.source === undefined ? {} : { source: q.source }),
});

/** Reads what the list shows at `search`, for the entry's loader. */
export const loadErrorList = (data: AppData, search: StaffSearch): Promise<void> =>
  loadErrorGroups(data, errorQueryOf(search));

/** Reads the group of `id`, for the route's loader. */
export const loadError = (data: AppData, id: string): Promise<void> => loadErrorGroup(data, id);

/** The filters of the list: the state and the source. */
function ErrorFilters({ q, onChange }: { q: ErrorGroupQuery; onChange: (q: ErrorGroupQuery) => void }) {
  const { t } = useStaffStrings();
  const states: FilterValue[] = errorGroupStates.map((state) => ({
    id: state,
    label: t("staff_operations.error_groups.state", { state }),
    icon: state === "open" ? "stateOpen" : state === "resolved" ? "stateResolved" : "stateMuted",
  }));
  const sources: FilterValue[] = errorGroupSources.map((source) => ({
    id: source,
    label: t("staff_operations.error_groups.source", { source }),
    icon: source === "server" ? "admin" : "apps",
  }));
  return (
    <FilterBar
      label={t("staff_operations.error_groups.filters")}
      applied={q.state !== "open" || q.source !== undefined}
      onClear={() => {
        onChange({ state: "open" });
      }}
    >
      <FilterFacet
        label={t("staff_operations.error_groups.column.state")}
        any={t("staff_operations.error_groups.state.any")}
        values={states}
        chosen={q.state}
        onChoose={(state) => {
          onChange({ ...q, state: errorGroupStates.find((s) => s === state) });
        }}
      />
      <FilterFacet
        label={t("staff_operations.error_groups.column.source")}
        any={t("staff_operations.error_groups.source.any")}
        values={sources}
        chosen={q.source}
        onChoose={(source) => {
          onChange({ ...q, source: errorGroupSources.find((s) => s === source) });
        }}
      />
    </FilterBar>
  );
}

type GroupColumn = "state" | "day" | "week" | "seen";

/** The groups of a state and a source, a page at a time, each leading to its screen. */
export function ErrorGroupsScreen({ search, onSearch }: StaffAddress): ReactNode {
  const { t } = useStaffStrings();
  const count = useCount();
  const q = errorQueryOf(search);
  const page = useErrorGroups(q);
  const head = (
    <>
      <ScreenTitle glyph="errors">{t("staff_operations.entry.error_groups")}</ScreenTitle>
      <ErrorFilters
        q={q}
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
          loading={t("staff_operations.error_groups.loading")}
          failed={t("staff_operations.error_groups.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<GroupColumn>[] = [
    { key: "day", label: t("staff_operations.error_groups.column.day"), kind: "figures" },
    { key: "week", label: t("staff_operations.error_groups.column.week"), kind: "figures" },
    { key: "seen", label: t("staff_operations.error_groups.column.seen"), kind: "fit", bare: true },
    { key: "state", label: t("staff_operations.error_groups.column.state"), kind: "fit", beside: true },
  ];
  // What happened is the row's line, where the eye lands; where it happened under it.
  const rows = page.items.map((g): RecordRow<GroupColumn> => ({
    id: g.id,
    head: g.message === "" ? g.type : g.message,
    code: true,
    // Where it happened, across the row under its cells, where a route has the room to break only at its slashes.
    detail: (
      <Box as="p" text="caption" tone="muted">
        <CodeText>{g.where}</CodeText>
      </Box>
    ),
    href: staffHref(errorGroupPath(g.id), search),
    cells: {
      day: count(g.count_24h),
      week: count(g.count_7d),
      seen: <RelativeTime date={g.last_seen_at} />,
      state: <GroupStateMark state={g.state} />,
    },
  }));
  return (
    <ScreenStack busy={page.stale}>
      {head}
      <RecordTable
        label={t("staff_operations.error_groups.table")}
        head={{ label: t("staff_operations.error_groups.column.type") }}
        columns={columns}
        rows={rows}
        busy={page.stale}
        empty={
          <FeedNotice start>
            {t(
              q.state === "open" && q.source === undefined
                ? "staff_operations.error_groups.empty"
                : "staff_operations.error_groups.empty.filtered",
            )}
          </FeedNotice>
        }
      />
      <Pages address={{ search, onSearch }} next={page.next} />
    </ScreenStack>
  );
}

/** The title of the screen of the group of `id`: its type, once read. */
export function useErrorGroupTitle(id: string): string | undefined {
  return useErrorGroup(id).value?.type;
}

/** The facts of a group: its state since when, where and what, when and on which version it was first and last seen. */
function groupFacts(t: T, moment: (iso: string) => string, g: ErrorGroupDetail): Fact[] {
  return [
    { term: t("staff_operations.error_groups.column.state"), value: <GroupStateMark state={g.state} /> },
    { term: t("staff_operations.error_groups.since"), value: moment(g.state_changed_at) },
    ...(g.resolved_version === undefined
      ? []
      : [{ term: t("staff_operations.error_groups.resolved_version"), value: g.resolved_version, code: true }]),
    {
      term: t("staff_operations.error_groups.column.source"),
      value: t("staff_operations.error_groups.source", { source: g.source }),
    },
    {
      term: t("staff_operations.error_groups.kind"),
      value: t("staff_operations.error_groups.kind.value", { kind: g.kind }),
    },
    { term: t("staff_operations.error_groups.where"), value: g.where, code: true },
    { term: t("staff_operations.error_groups.message"), value: g.message, code: true },
    { term: t("staff_operations.error_groups.first"), value: moment(g.first_seen_at) },
    { term: t("staff_operations.error_groups.first_version"), value: g.first_version, code: true },
    { term: t("staff_operations.error_groups.column.seen"), value: moment(g.last_seen_at) },
    { term: t("staff_operations.error_groups.last_version"), value: g.last_version, code: true },
    {
      term: t("staff_operations.error_groups.fingerprint"),
      value: g.fingerprint,
      code: true,
      action: <CopyAction label={t("staff_operations.copy.fingerprint")} value={g.fingerprint} />,
    },
  ];
}

/** The occurrences of the last 24 hours and 7 days, a table a screen reader reads. */
function Counts({ group }: { group: ErrorGroup }): ReactNode {
  const { t } = useStaffStrings();
  const count = useCount();
  return (
    <RecordTable
      label={t("staff_operations.error_groups.counts")}
      head={{ label: t("staff_operations.error_groups.counts.period") }}
      columns={[{ key: "count", label: t("staff_operations.error_groups.counts.count"), kind: "figures" }]}
      rows={[
        { id: "day", head: t("staff_operations.error_groups.column.day"), cells: { count: count(group.count_24h) } },
        { id: "week", head: t("staff_operations.error_groups.column.week"), cells: { count: count(group.count_7d) } },
      ]}
    />
  );
}

/**
 * The latest occurrence: its facts, the IDs the logs and the traces are searched by each with the button that copies
 * it, its message where it differs from the group's, and its stack to copy.
 */
function Sample({ sample, message }: { sample: NonNullable<ErrorGroupDetail["sample"]>; message: string }): ReactNode {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const id = (term: string, value: string | undefined, copy: string): Fact[] =>
    value === undefined ? [] : [{ term, value, code: true, action: <CopyAction label={copy} value={value} /> }];
  return (
    <Titled title={t("staff_operations.error_groups.sample")}>
      <Stack gap="entry-room">
        <FactList
          label={t("staff_operations.error_groups.sample")}
          items={[
            { term: t("staff_operations.error_groups.sample.at"), value: moment(sample.occurred_at) },
            { term: t("staff_operations.error_groups.sample.version"), value: sample.version, code: true },
            { term: t("staff_operations.error_groups.sample.role"), value: sample.role, code: true },
            { term: t("staff_operations.error_groups.sample.type"), value: sample.type, code: true },
            ...(sample.message === message
              ? []
              : [{ term: t("staff_operations.error_groups.message"), value: sample.message, code: true }]),
            ...id(
              t("staff_operations.error_groups.sample.request_id"),
              sample.request_id,
              t("staff_operations.copy.request_id"),
            ),
            ...id(
              t("staff_operations.error_groups.sample.trace_id"),
              sample.trace_id,
              t("staff_operations.copy.trace_id"),
            ),
          ]}
        />
        <CopyValue
          label={t("staff_operations.error_groups.sample.stack")}
          value={sample.stack}
          copy={t("staff_operations.copy.stack")}
          code
        />
      </Stack>
    </Titled>
  );
}

/** How many hours of occurrences show at first, and how many more each Show more adds. */
const hoursShown = 12;

/** The hours that had occurrences, the newest first, a table of figures: when, how many, on which version. */
function Hours({ group }: { group: ErrorGroupDetail }): ReactNode {
  const { t } = useStaffStrings();
  const count = useCount();
  const moment = useMoment();
  const [shown, setShown] = useState(hoursShown);
  const hours = group.hourly_counts.toReversed();
  const title = t("staff_operations.error_groups.hours");
  return (
    <Titled title={title}>
      {hours.length === 0 ? (
        <FeedNotice start>{t("staff_operations.error_groups.hours.empty")}</FeedNotice>
      ) : (
        <Stack gap="entry-room">
          <FinanceTable
            label={title}
            head={t("staff_operations.error_groups.hours.hour")}
            columns={[
              { label: t("staff_operations.error_groups.counts.count"), figures: true },
              { label: t("staff_operations.error_groups.sample.version") },
            ]}
          >
            {hours.slice(0, shown).map((h) => (
              <FinanceRow key={h.hour} head={moment(h.hour)} cells={[{ figures: [count(h.count)] }, h.last_version]} />
            ))}
          </FinanceTable>
          {hours.length > shown && (
            <MoreRecords
              onPress={() => {
                setShown((n) => n + hoursShown);
              }}
            />
          )}
        </Stack>
      )}
    </Titled>
  );
}

/** A change of a group's state, as its log says it. */
function GroupEvent({ event }: { event: ErrorGroupEvent }): ReactNode {
  const { t } = useStaffStrings();
  return (
    <LogEntry
      icon={event.kind === "regressed" ? "error" : event.kind === "resolved" ? "stateResolved" : "stateMuted"}
      date={event.occurred_at}
      about={
        event.version === undefined
          ? undefined
          : t("staff_operations.error_groups.event.version", { version: event.version })
      }
    >
      {t("staff_operations.error_groups.event", { kind: event.kind, actor: event.actor })}
    </LogEntry>
  );
}

/** What the reader may do with a group in its state. */
type Act = "resolve" | "mute" | "unmute";
const actsOf: Readonly<Record<ErrorGroupState, readonly Act[]>> = {
  open: ["resolve", "mute"],
  resolved: ["mute"],
  muted: ["unmute"],
};

/** A group: what the reader does with it, its facts, its counts, its latest occurrence, its hours and its history. */
function GroupView({ group, onBack }: { group: ErrorGroupDetail; onBack: () => void }): ReactNode {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const writes = useErrorGroupWrites();
  const [busy, setBusy] = useState<Act | undefined>(undefined);
  const { say, status } = useSaid();
  const act = (which: Act) => {
    if (busy !== undefined) return;
    setBusy(which);
    writes[which](group.id).then(
      () => {
        setBusy(undefined);
        say(t("staff_operations.error_groups.done", { act: which }));
      },
      (error: unknown) => {
        setBusy(undefined);
        say(refusalDetail(error) ?? t("staff_operations.error_groups.act_failed"), true);
      },
    );
  };
  return (
    <ScreenStack>
      <SubTitle glyph="errors" back={t("staff_operations.entry.error_groups")} onBack={onBack}>
        {group.type}
      </SubTitle>
      {/* Resolve is the one primary action; muting and unmuting are neutral, whatever the state. */}
      <Row wrap align="center" gap="screen-tab-gap">
        {actsOf[group.state].map((which) => (
          <Button
            key={which}
            kind={which === "resolve" ? "primary" : "secondary"}
            size="timeline"
            busy={busy === which}
            onPress={() => {
              act(which);
            }}
          >
            {t("staff_operations.error_groups.act", { act: which })}
          </Button>
        ))}
        {status}
      </Row>
      <FactList label={t("staff_operations.error_groups.facts")} items={groupFacts(t, moment, group)} />
      <Titled title={t("staff_operations.error_groups.counts")}>
        <Counts group={group} />
      </Titled>
      {group.sample !== undefined && <Sample sample={group.sample} message={group.message} />}
      <Hours group={group} />
      <Titled title={t("staff_operations.error_groups.events")}>
        {group.events.length === 0 ? (
          <FeedNotice start>{t("staff_operations.error_groups.events.empty")}</FeedNotice>
        ) : (
          <LogList label={t("staff_operations.error_groups.events")}>
            {group.events.toReversed().map((e, i) => (
              <GroupEvent key={`${e.occurred_at}-${String(i)}`} event={e} />
            ))}
          </LogList>
        )}
      </Titled>
    </ScreenStack>
  );
}

/** The group of `id`, or why it shows none. */
export function ErrorGroupScreen({
  id,
  search,
  onBack,
}: {
  id: string;
  search: StaffSearch;
  onBack: () => void;
}): ReactNode {
  const { t } = useStaffStrings();
  const read = useErrorGroup(id);
  if (read.missing)
    return (
      <FeedNotice
        alert
        lead
        action={<TextLink href={staffHref(errorsPath, search)}>{t("staff_operations.entry.error_groups")}</TextLink>}
      >
        {t("staff_operations.error_groups.missing")}
      </FeedNotice>
    );
  if (read.forbidden) return <FeedNotice lead>{t("staff_operations.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_operations.error_groups.one.loading")}
        failed={t("staff_operations.error_groups.one.failed")}
        retry={read.refetch}
      />
    );
  return <GroupView group={read.value} onBack={onBack} />;
}
