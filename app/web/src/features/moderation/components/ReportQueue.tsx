// SPDX-License-Identifier: AGPL-3.0-only
// The report queue below its heading (FR-701, FR-710): the reports that await the moderators and those handled,
// newest first, in the admin mode's list form: a toolbar with the state filter (kept in the URL by the route,
// web-client/README.md S3), a table of one page, and previous / next. Each row's first cell links to the report's page,
// where it is handled. An open report shows its category, how long it has waited and who handles it as badges
// (ADR-0110 P3), so that the grave and the old ones stand out.
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
import { type Report, type ReportFilter, reportFilters } from "../api/reports";
import { handleOf, isolate, reportOutcomeLabels, useReports } from "../hooks/useReports";
import { DateTime, RecordTable, RequirePermission } from "./parts";
import { AccountLabel, ReportAssigneeBadge, ReportCategoryBadge, ReportWaitBadge } from "./reportParts";

const filterLabels: Readonly<Record<ReportFilter, ClientMessageKey>> = {
  open: "client:admin.reports.filter.open",
  resolved: "client:admin.reports.filter.resolved",
};

function isFilter(key: unknown): key is ReportFilter {
  return reportFilters.some((filter) => filter === key);
}

export type ReportQueueProps = {
  /** The selected state, from the URL. */
  filter: ReportFilter;
  /** Selects another state: the route puts it into the URL. */
  onFilterChange: (filter: ReportFilter) => void;
};

export function ReportQueue(props: ReportQueueProps) {
  return (
    <RequirePermission permission="manage_reports">
      <ReportsView {...props} />
    </RequirePermission>
  );
}

function ReportsView({ filter, onFilterChange }: ReportQueueProps) {
  const t = useT();
  return (
    <Stack gap="4">
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.reports.filter.label")}
          value={filter}
          onChange={(key) => {
            if (isFilter(key)) onFilterChange(key);
          }}
        >
          {reportFilters.map((item) => (
            <SelectItem key={item} id={item}>
              {t(filterLabels[item])}
            </SelectItem>
          ))}
        </Select>
      </ListToolbar>
      {/* Another state is another list: it starts at its first page. */}
      <ReportList key={filter} filter={filter} />
    </Stack>
  );
}

function ReportList({ filter }: { filter: ReportFilter }) {
  const t = useT();
  const list = useReports(filter);
  const resolved = filter === "resolved";
  const empty = resolved ? (
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
      caption={t(filterLabels[filter])}
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
