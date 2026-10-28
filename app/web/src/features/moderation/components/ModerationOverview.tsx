// SPDX-License-Identifier: AGPL-3.0-only
// The moderation overview (`/moderation`, ADR-0058, ADR-0110 P1 and P3; screens/moderation.md): what waits for the
// moderators and what to handle first, not a copy of the mode's menu. "Waiting for you" is a tile for each queue that
// the viewer's roles count (FR-712), each a link to its queue, and beside the open reports how long the oldest of them
// has waited; "Reports to handle first" lists the open reports of the whole queue, the oldest legal ones first and then
// the longest waiting, each with its category, its wait and who handles it. A viewer whose roles allow no moderation
// screen is told why.
import { Cluster, EmptyState, Link, Section, Stack, StatTile, StatTileGrid, Text } from "@my/ui";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Fragment } from "react";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useNow } from "../../../lib/clock";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { AdminCount } from "../api/counts";
import { type Report, reportsQueryOptions } from "../api/reports";
import { useModerationCounts } from "../hooks/useModerationCounts";
import { useHasPermission } from "../hooks/usePermissions";
import { handleOf, isolate } from "../hooks/useReports";
import { reportWaitThresholds, waitLevel, waitUnits } from "../hooks/useWait";
import { moderationPages } from "./moderationPages";
import { ReportAssigneeBadge, ReportCategoryBadge, ReportWaitBadge } from "./reportParts";
import { useShownStaffPages } from "./StaffPages";

/** The queues the overview counts, in the order of their tiles, with the screen each tile leads to. */
const queues: readonly { count: AdminCount; label: ClientMessageKey; href: string }[] = [
  { count: "open_reports", label: "client:admin.overview.awaiting.open_reports", href: "/moderation/reports" },
  {
    count: "pending_registrations",
    label: "client:admin.overview.awaiting.pending_registrations",
    href: "/moderation/registrations",
  },
  { count: "pending_appeals", label: "client:admin.overview.awaiting.pending_appeals", href: "/moderation/appeals" },
];

/** How many reports "Reports to handle first" lists at most. */
const firstReportsShown = 5;

const reportWaitValues = {
  minutes: "client:admin.overview.awaiting.report_wait.minutes",
  hours: "client:admin.overview.awaiting.report_wait.hours",
  days: "client:admin.overview.awaiting.report_wait.days",
} as const;

/** The queue from the report that has waited longest, which the wait tile opens. */
const oldestReportsHref = "/moderation/reports?order=oldest";

export function ModerationOverview() {
  const t = useT();
  const shown = useShownStaffPages(moderationPages);
  const reports = useHasPermission("manage_reports");
  if (shown.length === 0) {
    return (
      <EmptyState
        heading={t("client:admin.moderation.none.heading")}
        description={t("client:admin.moderation.none.description")}
      />
    );
  }
  return (
    <Stack gap="8">
      <Waiting />
      {reports && <FirstReports />}
    </Stack>
  );
}

/** A tile for each queue the viewer's roles count, with the number that waits there. */
function Waiting() {
  const t = useT();
  const intl = useIntl();
  const counts = useModerationCounts();
  const counted = queues.filter((queue) => counts[queue.count] !== undefined);
  return (
    <Section title={t("client:admin.overview.awaiting.title")}>
      {counted.length === 0 ? (
        <Text tone="muted">{t("client:admin.overview.awaiting.unavailable")}</Text>
      ) : (
        <StatTileGrid>
          {counted.map((queue) => {
            const count = counts[queue.count] ?? 0;
            return (
              <Fragment key={queue.count}>
                <StatTile
                  as="li"
                  label={t(queue.label)}
                  value={intl.formatNumber(count)}
                  href={queue.href}
                  {...(count > 0 && { status: "warning", statusLabel: t("client:admin.overview.awaiting.status") })}
                />
                {queue.count === "open_reports" && <ReportWaitTile oldest={counts.oldest_open_report_filed_at} />}
              </Fragment>
            );
          })}
        </StatTileGrid>
      )}
    </Section>
  );
}

/** How long the open report that has waited longest has waited (W2: a length, not a date), from the counts. */
function ReportWaitTile({ oldest }: { oldest: string | undefined }) {
  const t = useT();
  const now = useNow();
  const label = t("client:admin.overview.awaiting.report_wait");
  if (oldest === undefined) {
    return (
      <StatTile
        as="li"
        label={label}
        value={t("client:admin.overview.awaiting.report_wait.none")}
        href={oldestReportsHref}
      />
    );
  }
  const waited = now - Date.parse(oldest);
  const { unit, count } = waitUnits(waited);
  const level = waitLevel(waited, reportWaitThresholds);
  const status =
    level === "fresh"
      ? {}
      : ({
          status: level === "overdue" ? "danger" : "warning",
          statusLabel: t("client:admin.overview.awaiting.report_wait.status"),
        } as const);
  return (
    <StatTile as="li" label={label} value={t(reportWaitValues[unit], { count })} href={oldestReportsHref} {...status} />
  );
}

/** The oldest open legal reports first (the law may set a deadline), then the longest waiting of the whole queue. */
function FirstReports() {
  const t = useT();
  const api = useApi();
  const legal = useInfiniteQuery(reportsQueryOptions(api, { state: "open", category: "legal", order: "oldest" }));
  const oldest = useInfiniteQuery(reportsQueryOptions(api, { state: "open", order: "oldest" }));
  const pending = legal.isPending || oldest.isPending;
  const loaded = [...(legal.data?.pages[0]?.items ?? []), ...(oldest.data?.pages[0]?.items ?? [])];
  const first = loaded
    .filter((report, index) => loaded.findIndex((other) => other.id === report.id) === index)
    .slice(0, firstReportsShown);
  return (
    <Section
      title={t("client:admin.overview.first_reports.title")}
      description={t("client:admin.overview.first_reports.description")}
      busy={pending}
    >
      {legal.isError || oldest.isError ? (
        <Text tone="muted">{t("client:admin.overview.first_reports.unavailable")}</Text>
      ) : pending ? null : first.length === 0 ? (
        <EmptyState headingLevel={3} heading={t("client:admin.reports.empty.open")} />
      ) : (
        <Stack as="ul" divided border="lg">
          {first.map((report) => (
            <FirstReport key={report.id} report={report} />
          ))}
        </Stack>
      )}
      <Link href="/moderation/reports" variant="standalone">
        {t("client:admin.overview.first_reports.all")}
      </Link>
    </Section>
  );
}

function FirstReport({ report }: { report: Report }) {
  const t = useT();
  return (
    <Stack as="li" gap="2" paddingX="4" paddingY="3">
      <Link href={`/moderation/reports/${encodeURIComponent(report.id)}`} variant="standalone" wrap="anywhere">
        {report.target_account === undefined
          ? t("client:admin.report.of_deleted")
          : t("client:admin.report.of", { target: isolate(handleOf(report.target_account)) })}
      </Link>
      <Cluster gap="2" align="center">
        <ReportCategoryBadge category={report.category} />
        <ReportWaitBadge filedAt={report.filed_at} />
        <ReportAssigneeBadge id={report.assignee_account_id} />
      </Cluster>
    </Stack>
  );
}
