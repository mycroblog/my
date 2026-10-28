// SPDX-License-Identifier: AGPL-3.0-only
// How long the job queue keeps its oldest due job waiting (ADR-0110 W2: a length, not a date), and whether that is
// backed up: the "Longest job wait" tile of the administration overview and of the jobs screen.
import { useNow } from "../../../lib/clock";
import { useT } from "../../../lib/i18n";
import { type WaitThresholds, waitLevel, waitUnits } from "../../moderation";

const minute = 60_000;

/** The queue is backed up when it keeps a job waiting this long, and failing when this long. */
const jobWait: WaitThresholds = { aging: 15 * minute, overdue: 60 * minute };

const jobWaitValues = {
  minutes: "client:admin.overview.health.job_wait.minutes",
  hours: "client:admin.overview.health.job_wait.hours",
  days: "client:admin.overview.health.job_wait.days",
} as const;

/** The wait as a tile shows it: its value in words, and a status when the queue is backed up. */
export type JobWaitView = { value: string; status?: "warning" | "danger"; statusLabel?: string };

/** The wait of the job due at `oldestReadyAt` (ISO 8601), or no wait when it is undefined. */
export function useJobWait(oldestReadyAt: string | undefined): JobWaitView {
  const t = useT();
  const now = useNow();
  if (oldestReadyAt === undefined) return { value: t("client:admin.overview.health.job_wait.none") };
  const waited = now - Date.parse(oldestReadyAt);
  const { unit, count } = waitUnits(waited);
  const level = waitLevel(waited, jobWait);
  const value = t(jobWaitValues[unit], { count });
  if (level === "fresh") return { value };
  return {
    value,
    status: level === "overdue" ? "danger" : "warning",
    statusLabel: t("client:admin.overview.health.job_wait.status"),
  };
}
