// SPDX-License-Identifier: AGPL-3.0-only
// The server administration overview (`/admin`, ADR-0058, ADR-0110 P1 and P3; screens/admin.md): whether the server
// runs well, not a copy of the mode's menu. "Server health" has a tile for each thing the viewer's roles watch, each a
// link to its screen: the failed jobs and how long the queue keeps a job waiting (manage_jobs), and the open error
// groups (manage_errors). A tile that needs attention says so in a word beside a warning icon. A viewer whose roles
// watch none of them is pointed to the menu; one whose roles allow no administration screen is told why.
import { EmptyState, Section, StatTile, StatTileGrid, Text } from "@my/ui";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import { useHasPermission, useShownStaffPages } from "../../moderation";
import { errorGroupsQueryOptions } from "../api/errors";
import { jobQueueQueryOptions } from "../api/jobs";
import { useJobWait } from "../hooks/useJobWait";
import { adminPages } from "./AdminMenu";

export function AdminOverview() {
  const t = useT();
  const shown = useShownStaffPages(adminPages);
  const jobs = useHasPermission("manage_jobs");
  const errors = useHasPermission("manage_errors");
  if (shown.length === 0) {
    return (
      <EmptyState heading={t("client:admin.home.none.heading")} description={t("client:admin.home.none.description")} />
    );
  }
  if (!jobs && !errors) return <Text tone="muted">{t("client:admin.overview.health.none")}</Text>;
  return (
    <Section title={t("client:admin.overview.health.title")}>
      <StatTileGrid>
        {jobs && <FailedJobsTile />}
        {jobs && <JobWaitTile />}
        {errors && <OpenErrorsTile />}
      </StatTileGrid>
    </Section>
  );
}

/** The jobs that failed for good, which wait for an administrator to run them again. */
function FailedJobsTile() {
  const t = useT();
  const intl = useIntl();
  const api = useApi();
  const queue = useQuery(jobQueueQueryOptions(api));
  if (queue.data === undefined)
    return <PendingTile label={t("client:admin.overview.health.failed_jobs")} failed={queue.isError} />;
  const dead = queue.data.counts.filter((count) => count.state === "dead").reduce((sum, count) => sum + count.count, 0);
  return (
    <StatTile
      as="li"
      label={t("client:admin.overview.health.failed_jobs")}
      value={intl.formatNumber(dead)}
      href="/admin/jobs?state=dead"
      {...(dead > 0 && { status: "danger", statusLabel: t("client:admin.overview.health.failed_jobs.status") })}
    />
  );
}

/** How long the queue has kept its oldest due job waiting (W2: a length, not a date). */
function JobWaitTile() {
  const t = useT();
  const api = useApi();
  const queue = useQuery(jobQueueQueryOptions(api));
  const label = t("client:admin.overview.health.job_wait");
  const wait = useJobWait(queue.data?.oldest_ready_at);
  if (queue.data === undefined) return <PendingTile label={label} failed={queue.isError} />;
  return <StatTile as="li" label={label} href="/admin/jobs" {...wait} />;
}

/** The error groups that are open: the first page's, with a "+" when there are more. */
function OpenErrorsTile() {
  const t = useT();
  const intl = useIntl();
  const api = useApi();
  const groups = useInfiniteQuery(errorGroupsQueryOptions(api, { state: "open", source: "all" }));
  const label = t("client:admin.overview.health.open_errors");
  const page = groups.data?.pages[0];
  if (page === undefined) return <PendingTile label={label} failed={groups.isError} />;
  const count = page.items.length;
  const more = page.next_cursor !== undefined;
  return (
    <StatTile
      as="li"
      label={label}
      value={more ? t("client:admin.overview.health.open_errors.more", { count }) : intl.formatNumber(count)}
      href="/admin/errors"
      {...(count > 0 && { status: "warning", statusLabel: t("client:admin.overview.health.open_errors.status") })}
    />
  );
}

/** A tile whose number is loading, or could not be loaded. */
function PendingTile({ label, failed }: { label: string; failed: boolean }) {
  const t = useT();
  return (
    <StatTile
      as="li"
      label={label}
      value={t(failed ? "client:admin.overview.value.unavailable" : "client:admin.overview.value.loading")}
    />
  );
}
