// SPDX-License-Identifier: AGPL-3.0-only

// The moderators' queue of reports, at /moderation/reports (FR-709–FR-712): the open reports by default, the newest
// first, filtered by their state, their category and who handles them (the reader, or nobody), which the address
// keeps; each report its account, who filed it, its category, when, and its state with who handles it; a page at a
// time by the server's cursor, Previous going back along the pages the reader came by. The column beside a report's
// screen shows the same page, compact (`ReportQueueBeside`). Where the queue has nothing to show, it says why.

import { reportPath, reportQueuePath, sentenceHandleOf } from "@my/app-components";
import {
  reportCategories,
  useReportAccount,
  useReport,
  useReportQueue,
  useViewer,
  type QueuedReport,
  type ReportQueueQuery,
} from "@my/app-data";
import {
  Pager,
  FeedNotice,
  FilterBar,
  FilterFacet,
  FrameSection,
  RecordTable,
  RelativeTime,
  ScreenStack,
  ScreenTitle,
  StateMark,
  type FilterValue,
  type RecordColumn,
  type RecordRow,
  type RecordState,
} from "@my/design-system";
import type { ReactNode } from "react";
import { staffHref, type StaffAddress, type StaffSearch } from "../entries";
import { cursorOf, useAddressPages, ReadNotice, useStaffStrings, type T } from "../parts";

type Category = (typeof reportCategories)[number];

/** What the queue shows at an address's query: the open reports, the newest first, unless it says otherwise. */
export function queueOf(search: StaffSearch): ReportQueueQuery {
  const { state, category, assignee, order } = search;
  return {
    state: state === "any" ? undefined : state === "resolved" ? "resolved" : "open",
    category: reportCategories.find((c) => c === category),
    assignee: assignee === "me" || assignee === "none" ? assignee : undefined,
    order: order === "oldest" ? "oldest" : "newest",
    cursor: cursorOf(search),
  };
}

/** The query of the queue's address for `q`, at its first page. */
function searchOf(q: ReportQueueQuery): StaffSearch {
  return {
    ...(q.state === "open" ? {} : { state: q.state ?? "any" }),
    ...(q.category === undefined ? {} : { category: q.category }),
    ...(q.assignee === undefined ? {} : { assignee: q.assignee }),
    ...(q.order === "oldest" ? { order: "oldest" } : {}),
    ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
  };
}

/** The address of the report of `id`, which keeps the queue's query, so that its screen goes back to the same page. */
export const reportHref = (id: string, search: StaffSearch): string => staffHref(reportPath(id), search);

/** The address of the queue at `search`. */
export const queueHref = (search: StaffSearch): string => staffHref(reportQueuePath, search);

/** Whether `q` filters anything the queue does not filter at first (the open reports, of every kind). */
const filtered = (q: ReportQueueQuery) => q.state !== "open" || q.category !== undefined || q.assignee !== undefined;

/** The state of a report as its row says it: open, assigned (to the reader, or to whom), or resolved. */
function ReportStateMark({ report }: { report: QueuedReport }): ReactNode {
  const { t } = useStaffStrings();
  const me = useViewer()?.id;
  const assignee = useReportAccount(report.assignee_account_id);
  const [state, words]: [RecordState, string] =
    report.state === "resolved"
      ? ["resolved", t("staff.report.state.resolved")]
      : report.assignee_account_id === undefined
        ? ["open", t("staff.report.state.open")]
        : report.assignee_account_id === me
          ? ["assigned", t("staff.report.state.assigned_you")]
          : [
              "assigned",
              assignee === undefined
                ? t("staff.report.state.assigned_someone")
                : t("staff.report.state.assigned", { handle: sentenceHandleOf(assignee) }),
            ];
  return <StateMark state={state}>{words}</StateMark>;
}

/** The category of a report, in words. */
export const categoryWords = (t: T, category: Category): string => t("staff.report.category", { category });

/** Who filed a report, as its row's line under its account says it. */
function filedBy(t: T, report: QueuedReport): string {
  if (report.hook_origin !== undefined) return t("staff.reports.by_review");
  if (report.uri !== undefined) {
    const domain = report.reporter_account?.acct.split("@")[1] ?? new URL(report.uri).host;
    return t("staff.reports.by_server", { domain });
  }
  return report.reporter_account === undefined
    ? t("staff.reports.by_gone")
    : t("staff.reports.by", { handle: sentenceHandleOf(report.reporter_account) });
}

type Column = "category" | "filed" | "state";

/** The queue's table: a row a report, opening its screen at `href`. */
function QueueTable({
  reports,
  search,
  compact,
  current,
  busy,
  onOrder,
  empty,
}: {
  reports: readonly QueuedReport[];
  search: StaffSearch;
  compact: boolean;
  current?: string | undefined;
  busy?: boolean | undefined;
  onOrder?: ((order: "newest" | "oldest") => void) | undefined;
  empty?: ReactNode;
}) {
  const { t } = useStaffStrings();
  const me = useViewer()?.id;
  const q = queueOf(search);
  const columns: RecordColumn<Column>[] = compact
    ? [{ key: "state", label: t("staff.reports.column.state"), kind: "fit", beside: true }]
    : [
        { key: "category", label: t("staff.reports.column.category"), bare: true },
        { key: "filed", label: t("staff.reports.column.filed"), kind: "fit", sorts: "descending", bare: true },
        { key: "state", label: t("staff.reports.column.state"), kind: "fit", beside: true },
      ];
  const rows = reports.map((r): RecordRow<Column> => ({
    id: r.id,
    head: r.target_account === undefined ? t("staff.account_gone") : sentenceHandleOf(r.target_account),
    // Beside a report, two reports of one account are told apart by their categories.
    // Who reported a moderator is never shown to them (ADR-0058 §9).
    caption: compact ? categoryWords(t, r.category) : r.target_account_id === me ? undefined : filedBy(t, r),
    href: reportHref(r.id, search),
    cells: {
      category: categoryWords(t, r.category),
      filed: <RelativeTime date={r.filed_at} />,
      state: <ReportStateMark report={r} />,
    },
  }));
  return (
    <RecordTable
      label={t("staff.reports.table")}
      head={{ label: t("staff.reports.column.account") }}
      columns={columns}
      rows={rows}
      sort={compact ? undefined : { by: "filed", direction: q.order === "oldest" ? "ascending" : "descending" }}
      onSort={
        onOrder === undefined
          ? undefined
          : (sort) => {
              onOrder(sort.direction === "ascending" ? "oldest" : "newest");
            }
      }
      current={current}
      busy={busy}
      empty={empty}
      even={!compact}
      contained={compact}
    />
  );
}

/** The filters of the queue, over it. */
function QueueFilters({ q, onChange }: { q: ReportQueueQuery; onChange: (q: ReportQueueQuery) => void }) {
  const { t } = useStaffStrings();
  const states: FilterValue[] = [
    { id: "open", label: t("staff.reports.state.open"), icon: "stateOpen" },
    { id: "resolved", label: t("staff.reports.state.resolved"), icon: "stateResolved" },
  ];
  const categories: FilterValue[] = reportCategories.map((c) => ({
    id: c,
    label: categoryWords(t, c),
    icon: "report",
  }));
  const assignees: FilterValue[] = [
    { id: "me", label: t("staff.reports.assignee.me"), icon: "stateAssigned" },
    { id: "none", label: t("staff.reports.assignee.none"), icon: "stateOpen" },
  ];
  // A change of the filters starts at the first page, in the same order.
  const change = (next: Partial<ReportQueueQuery>) => {
    onChange({ ...q, ...next, cursor: undefined });
  };
  return (
    <FilterBar
      label={t("staff.reports.filters")}
      applied={filtered(q)}
      onClear={() => {
        onChange({ state: "open", order: q.order });
      }}
    >
      <FilterFacet
        label={t("staff.reports.state")}
        any={t("staff.reports.state.any")}
        values={states}
        chosen={q.state}
        onChoose={(state) => {
          change({ state: state === "resolved" ? "resolved" : state === "open" ? "open" : undefined });
        }}
      />
      <FilterFacet
        label={t("staff.reports.category")}
        any={t("staff.reports.category.any")}
        values={categories}
        chosen={q.category}
        onChoose={(category) => {
          change({ category: reportCategories.find((c) => c === category) });
        }}
      />
      <FilterFacet
        label={t("staff.reports.assignee")}
        any={t("staff.reports.assignee.any")}
        values={assignees}
        chosen={q.assignee}
        onChoose={(assignee) => {
          change({ assignee: assignee === "me" || assignee === "none" ? assignee : undefined });
        }}
      />
    </FilterBar>
  );
}

/** The queue of reports: its filters, the page of reports, and the pages. */
export function ReportQueueScreen({ search, onSearch }: StaffAddress): ReactNode {
  const pages = useAddressPages({ search, onSearch });
  const { t } = useStaffStrings();
  const q = queueOf(search);
  const page = useReportQueue(q);
  const go = (next: ReportQueueQuery) => {
    onSearch(searchOf(next));
  };
  const filters = (
    <>
      <ScreenTitle>{t("staff.entry.reports")}</ScreenTitle>
      <QueueFilters q={q} onChange={go} />
    </>
  );
  if (page.forbidden) return <FeedNotice lead>{t("staff.reports.forbidden")}</FeedNotice>;
  if (page.status !== "success")
    return (
      <ScreenStack>
        {filters}
        <ReadNotice
          status={page.status}
          loading={t("staff.reports.loading")}
          failed={t("staff.reports.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  return (
    <ScreenStack busy={page.stale}>
      {filters}
      <QueueTable
        reports={page.reports}
        search={search}
        compact={false}
        busy={page.stale}
        onOrder={(order) => {
          go({ ...q, order, cursor: undefined });
        }}
        empty={
          // The filters' own Clear filters, over the notice, is the way back.
          filtered(q) ? (
            <FeedNotice>{t("staff.reports.empty.filtered")}</FeedNotice>
          ) : (
            <FeedNotice>{t("staff.reports.empty.open")}</FeedNotice>
          )
        }
      />
      <Pager {...pages.pager(page.next)} />
    </ScreenStack>
  );
}

/**
 * The queue beside a report's screen, from the frame's `wide`, under its heading, which leads to it: the page the
 * reader came from, compact (each report's account, its category and its state), the report at `current` marked; a
 * report opened from no queue's page (a link, a notification) beside the first page of the reports in its state, where
 * it is. Nothing while it loads or could not be read, since the report's screen says so of its own read.
 */
export function ReportQueueBeside({ search, current }: { search: StaffSearch; current: string }): ReactNode {
  const { t } = useStaffStrings();
  const state = useReport(current).report?.state;
  const shown: StaffSearch = search["state"] === undefined && state === "resolved" ? { ...search, state } : search;
  const page = useReportQueue(queueOf(shown));
  if (page.status !== "success") return null;
  return (
    <FrameSection id="beside-reports" icon="moderation" href={queueHref(shown)} heading={t("staff.entry.reports")}>
      <QueueTable reports={page.reports} search={shown} compact current={current} />
    </FrameSection>
  );
}
