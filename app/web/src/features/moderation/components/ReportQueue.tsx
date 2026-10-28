// SPDX-License-Identifier: AGPL-3.0-only
// The report queue below its heading (FR-701, FR-710): the reports that await the moderators and those handled, in
// the admin mode's list form: a toolbar with the filters of the state, the category and the assignee, and the order
// (newest or oldest first; all kept in the URL by the route, web-client/README.md S3), a table of one page, and
// previous / next. Each row's first cell links to the report's page, where it is handled. An open report shows its
// category, how long it has waited and who handles it as badges (ADR-0110 P3), so that the grave and the old ones
// stand out.
import {
  EmptyState,
  Link,
  ListToolbar,
  Select,
  SelectItem,
  Stack,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
} from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  type Report,
  type ReportAssigneeFilter,
  reportAssigneeFilters,
  reportCategories,
  type ReportCategory,
  type ReportFilter,
  reportFilters,
  type ReportListQuery,
  type ReportOrder,
  reportOrders,
} from "../api/reports";
import { handleOf, isolate, reportCategoryLabels, reportOutcomeLabels, useReports } from "../hooks/useReports";
import { DateTime, RecordTable, RequirePermission } from "./parts";
import { AccountLabel, ReportAssigneeBadge, ReportCategoryBadge, ReportWaitBadge } from "./reportParts";

const filterLabels: Readonly<Record<ReportFilter, ClientMessageKey>> = {
  open: "client:admin.reports.filter.open",
  resolved: "client:admin.reports.filter.resolved",
};

const assigneeLabels: Readonly<Record<ReportAssigneeFilter, ClientMessageKey>> = {
  me: "client:admin.reports.filter.assignee.me",
  none: "client:admin.reports.filter.assignee.none",
};

const orderLabels: Readonly<Record<ReportOrder, ClientMessageKey>> = {
  newest: "client:admin.reports.filter.order.newest",
  oldest: "client:admin.reports.filter.order.oldest",
};

/** The option of a select that keeps every category or every assignee. */
const every = "all";

function isFilter(key: unknown): key is ReportFilter {
  return reportFilters.some((filter) => filter === key);
}

function isCategory(key: unknown): key is ReportCategory {
  return reportCategories.some((category) => category === key);
}

function isAssignee(key: unknown): key is ReportAssigneeFilter {
  return reportAssigneeFilters.some((assignee) => assignee === key);
}

function isOrder(key: unknown): key is ReportOrder {
  return reportOrders.some((order) => order === key);
}

export type ReportQueueProps = {
  /** What the queue lists, from the URL. */
  list: ReportListQuery;
  /** Selects other filters or another order: the route puts them into the URL. */
  onListChange: (list: ReportListQuery) => void;
};

export function ReportQueue(props: ReportQueueProps) {
  return (
    <RequirePermission permission="manage_reports">
      <ReportsView {...props} />
    </RequirePermission>
  );
}

function ReportsView({ list, onListChange }: ReportQueueProps) {
  const t = useT();
  return (
    <Stack gap="4">
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.reports.filter.label")}
          value={list.state}
          onChange={(key) => {
            if (isFilter(key)) onListChange({ ...list, state: key });
          }}
        >
          {reportFilters.map((item) => (
            <SelectItem key={item} id={item}>
              {t(filterLabels[item])}
            </SelectItem>
          ))}
        </Select>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.reports.filter.category")}
          value={list.category ?? every}
          onChange={(key) => {
            onListChange({ ...list, category: isCategory(key) ? key : undefined });
          }}
        >
          <SelectItem id={every}>{t("client:admin.reports.filter.category.all")}</SelectItem>
          {reportCategories.map((category) => (
            <SelectItem key={category} id={category}>
              {t(reportCategoryLabels[category])}
            </SelectItem>
          ))}
        </Select>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.reports.filter.assignee")}
          value={list.assignee ?? every}
          onChange={(key) => {
            onListChange({ ...list, assignee: isAssignee(key) ? key : undefined });
          }}
        >
          <SelectItem id={every}>{t("client:admin.reports.filter.assignee.all")}</SelectItem>
          {reportAssigneeFilters.map((assignee) => (
            <SelectItem key={assignee} id={assignee}>
              {t(assigneeLabels[assignee])}
            </SelectItem>
          ))}
        </Select>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.reports.filter.order")}
          value={list.order ?? "newest"}
          onChange={(key) => {
            if (isOrder(key)) onListChange({ ...list, order: key });
          }}
        >
          {reportOrders.map((order) => (
            <SelectItem key={order} id={order}>
              {t(orderLabels[order])}
            </SelectItem>
          ))}
        </Select>
      </ListToolbar>
      {/* Other filters or another order are another list: it starts at its first page. */}
      <ReportList key={JSON.stringify(list)} query={list} />
    </Stack>
  );
}

function ReportList({ query }: { query: ReportListQuery }) {
  const t = useT();
  const list = useReports(query);
  const resolved = query.state === "resolved";
  const filtered = query.category !== undefined || query.assignee !== undefined;
  const empty = filtered ? (
    <EmptyState headingLevel={2} heading={t("client:admin.reports.empty.filtered")} />
  ) : resolved ? (
    <EmptyState headingLevel={2} heading={t("client:admin.reports.empty.resolved")} />
  ) : (
    <EmptyState
      headingLevel={2}
      heading={t("client:admin.reports.empty.open")}
      description={t("client:admin.reports.empty.open.description")}
    />
  );
  return (
    <RecordTable
      list={list}
      caption={t(filterLabels[query.state])}
      getId={(report) => report.id}
      empty={empty}
      head={
        <>
          <TableColumnHeader>{t("client:admin.reports.column.report")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.report.category")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.report.reporter")}</TableColumnHeader>
          <TableColumnHeader>
            {t(resolved ? "client:admin.report.outcome" : "client:admin.report.assignee")}
          </TableColumnHeader>
          <TableColumnHeader>
            {t(resolved ? "client:admin.report.resolved_at" : "client:admin.reports.column.waiting")}
          </TableColumnHeader>
        </>
      }
    >
      {(report) => <ReportRow report={report} />}
    </RecordTable>
  );
}

function ReportRow({ report }: { report: Report }) {
  const t = useT();
  return (
    <TableRow data-row-id={report.id}>
      <TableRowHeader wrap="anywhere">
        <Stack gap="1">
          <Link href={`/moderation/reports/${encodeURIComponent(report.id)}`} variant="standalone" wrap="anywhere">
            {report.target_account === undefined
              ? t("client:admin.report.of_deleted")
              : t("client:admin.report.of", { target: isolate(handleOf(report.target_account)) })}
          </Link>
          <Text size="sm" tone="muted" numeric>
            {t("client:admin.report.posts.count", { count: report.status_ids.length })}
          </Text>
          {report.comment !== "" && (
            // Written by the reporter, in any language; the report's page shows it whole.
            <Text as="p" size="sm" dir="auto" lineClamp={2} align="start" wrap="anywhere" whitespace="preWrap">
              {report.comment}
            </Text>
          )}
        </Stack>
      </TableRowHeader>
      <TableCell>
        <ReportCategoryBadge category={report.category} />
      </TableCell>
      <TableCell wrap="anywhere">
        <AccountLabel account={report.reporter_account} />
      </TableCell>
      <TableCell wrap="anywhere">
        {report.state === "resolved" ? (
          report.outcome === undefined ? (
            t("client:admin.reports.filter.resolved")
          ) : (
            t(reportOutcomeLabels[report.outcome])
          )
        ) : (
          <ReportAssigneeBadge id={report.assignee_account_id} />
        )}
      </TableCell>
      <TableCell>
        {report.state === "resolved" ? (
          <DateTime value={report.state_changed_at} />
        ) : (
          <ReportWaitBadge filedAt={report.filed_at} />
        )}
      </TableCell>
    </TableRow>
  );
}
