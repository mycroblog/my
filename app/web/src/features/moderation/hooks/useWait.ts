// SPDX-License-Identifier: AGPL-3.0-only
// How long an item of a queue has waited (ADR-0110 P3): the words and the level of @my/ui's AgeBadge, from when it
// started to wait and the thresholds of its queue. The report queue and the moderation overview use the report
// thresholds; the job queue measures its own wait (the server administration overview).
import type { AgeLevel } from "@my/ui";
import { useIntl } from "react-intl";
import { useNow } from "../../../lib/clock";
import { useT } from "../../../lib/i18n";

const minute = 60_000;
const hour = 60 * minute;
const day = 24 * hour;

/** When an item's wait becomes `aging` and `overdue`, in ms. */
export type WaitThresholds = { aging: number; overdue: number };

/** A report waits a day before it is aging, and three days before it is overdue. */
export const reportWaitThresholds: WaitThresholds = { aging: day, overdue: 3 * day };

/** The wait in whole units: minutes under an hour, hours under a day, then days. */
export function waitUnits(elapsed: number): { unit: "minutes" | "hours" | "days"; count: number } {
  const ms = Math.max(0, elapsed);
  if (ms < hour) return { unit: "minutes", count: Math.floor(ms / minute) };
  if (ms < day) return { unit: "hours", count: Math.floor(ms / hour) };
  return { unit: "days", count: Math.floor(ms / day) };
}

/** The level of a wait of `elapsed` ms against `thresholds`. */
export function waitLevel(elapsed: number, thresholds: WaitThresholds): AgeLevel {
  if (elapsed >= thresholds.overdue) return "overdue";
  if (elapsed >= thresholds.aging) return "aging";
  return "fresh";
}

/** What an AgeBadge shows of a wait that started at `since` (ISO 8601). */
export type WaitView = { label: string; level: AgeLevel; dateTime: string; title: string };

const waitLabels = {
  minutes: "client:admin.reports.wait.minutes",
  hours: "client:admin.reports.wait.hours",
  days: "client:admin.reports.wait.days",
} as const;

/** The AgeBadge of a wait that started at `since`; it renders again as the clock ticks. */
export function useWaitView(since: string, thresholds: WaitThresholds): WaitView {
  const t = useT();
  const intl = useIntl();
  const now = useNow();
  const started = Date.parse(since);
  const elapsed = now - started;
  const { unit, count } = waitUnits(elapsed);
  return {
    label: t(waitLabels[unit], { count }),
    level: waitLevel(elapsed, thresholds),
    dateTime: since,
    title: intl.formatDate(started, { dateStyle: "full", timeStyle: "short" }),
  };
}
