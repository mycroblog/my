// SPDX-License-Identifier: AGPL-3.0-only
// The date and time fields of a schedule (web-client/screens/compose.md §4, drafts.md §4): a wall-clock date and time
// in the user's time zone (FR-007), checked against the instance's limits of a schedule before it is sent (the server
// checks them again, ADR-0156 §7). The new-post form and the drafts page share it.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useLocale, useT } from "../../../lib/i18n";
import { useDisplaySettings } from "../../../lib/locale";
import { shellQueryReader } from "../../../lib/query";
import { instanceQueryOptions } from "../api/compose";
import { effectiveTimeZone, instantOf, timeZoneName, type WallClock, wallClockOf } from "./zonedTime";

const minute = 60 * 1000;
const day = 24 * 60 * minute;

/** The instance's limits of a schedule when it states none: the server's defaults (ADR-0156 §7). */
const defaultLeads = { minMinutes: 5, maxDays: 366 };

/** The time a new schedule starts from: the next full hour that is at least the least lead time from `now`. */
export function defaultScheduleTime(now: number, minMinutes: number): Date {
  const hour = 60 * minute;
  return new Date(Math.ceil((now + minMinutes * minute) / hour) * hour);
}

export type ScheduleFields = WallClock & {
  setDate: (date: string) => void;
  setTime: (time: string) => void;
  /** The name of the time zone the fields are in, in the display language. */
  zoneName: string;
  /** Why the chosen time cannot be scheduled, or why the schedule failed (localized); null otherwise. */
  error: string | null;
  isPending: boolean;
  /** Checks the time and schedules it; `close` closes the popover once it is scheduled. */
  submit: (close: () => void) => void;
};

/**
 * The fields of a schedule that starts at `initial` (an ISO date-time), or at the next full hour. `check` says why
 * the draft cannot be scheduled at all (as the submit button would), and `schedule` sends the instant, resolving to
 * null once scheduled or to the localized reason it is not.
 */
export function useSchedule({
  initial,
  check = () => null,
  schedule,
}: {
  initial: string | null;
  check?: () => string | null;
  schedule: (at: Date) => Promise<string | null>;
}): ScheduleFields {
  const api = useApi();
  const t = useT();
  const locale = useLocale();
  const { timeZone } = useDisplaySettings();
  const zone = effectiveTimeZone(timeZone);
  const limits = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader }).data?.limits.drafts;
  const minMinutes = limits?.scheduled_post_min_lead_minutes ?? defaultLeads.minMinutes;
  const maxDays = limits?.scheduled_post_max_lead_days ?? defaultLeads.maxDays;
  // The fields start once, when the popover opens (the hook lives in its content).
  const [clock, setClock] = useState<WallClock>(() =>
    wallClockOf(initial === null ? defaultScheduleTime(Date.now(), minMinutes) : new Date(initial), zone),
  );
  const [error, setError] = useState<string | null>(null);
  const [isPending, setIsPending] = useState(false);
  const [openedAt] = useState(() => new Date());
  return {
    ...clock,
    setDate: (date) => {
      setClock((c) => ({ ...c, date }));
      setError(null);
    },
    setTime: (time) => {
      setClock((c) => ({ ...c, time }));
      setError(null);
    },
    zoneName: timeZoneName(zone, locale, openedAt),
    error,
    isPending,
    submit: (close) => {
      const problem = check();
      if (problem !== null) {
        setError(problem);
        return;
      }
      const at = instantOf(clock, zone);
      const now = Date.now();
      if (at === null) {
        setError(t("client:drafts.schedule.invalid"));
        return;
      }
      if (at.getTime() < now + minMinutes * minute) {
        setError(t("client:drafts.schedule.too_soon", { minutes: minMinutes }));
        return;
      }
      if (at.getTime() > now + maxDays * day) {
        setError(t("client:drafts.schedule.too_far", { days: maxDays }));
        return;
      }
      setIsPending(true);
      void schedule(at).then((reason) => {
        setIsPending(false);
        if (reason === null) close();
        else setError(reason);
      });
    },
  };
}
