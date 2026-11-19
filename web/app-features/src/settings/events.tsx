// SPDX-License-Identifier: AGPL-3.0-only

// Events, the entry of the settings at /settings/events (ADR-0161 §12, FR-947): the reminders the server sends before
// an event the reader answered going or maybe, up to three, each 5 minutes to 7 days before its start. Each is a
// choice that saves at once and stays in the place the reader chose it (the earliest first as the screen opens), None
// for a place that holds none; until the reader chooses, they are
// the defaults (one day and one hour before), which a press brings back once the reader chose others.

import { eventRemindersPath } from "@my/app-components";
import {
  eventLimits,
  eventRemindersQuery,
  useChangeEventReminders,
  useEventReminders,
  type AppData,
} from "@my/app-data";
import {
  Box,
  Button,
  SaveState,
  SettingsScreen,
  SettingsSection,
  Stack,
  useLocale,
  type SelectOption,
} from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../locale";
import { Choice } from "./controls";
import type { SettingsEntryProps } from "./entries";
import { ReadingNotice } from "./parts";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: the reminders, from the cache when it has it. */
export async function loadEventSettings(data: AppData): Promise<void> {
  await data.queryClient.query({ ...eventRemindersQuery(data), staleTime: Infinity }).catch(() => undefined);
}

const hour = 60;
const day = 24 * hour;

/** The times before the start a reminder offers, in minutes, the earliest first: 5 minutes to 7 days. */
const offered: readonly number[] = [
  7 * day,
  3 * day,
  2 * day,
  day,
  12 * hour,
  6 * hour,
  3 * hour,
  2 * hour,
  hour,
  30,
  15,
  10,
  5,
].filter((m) => m >= eventLimits.reminderMinutes.min && m <= eventLimits.reminderMinutes.max);

/** The value of the choice of no reminder. */
const none = "";

/** How long before the start `minutes` is, in the largest unit that says it whole. */
function before(minutes: number, t: T): string {
  if (minutes % day === 0) return t("settings.events.before", { unit: "day", count: minutes / day });
  if (minutes % hour === 0) return t("settings.events.before", { unit: "hour", count: minutes / hour });
  return t("settings.events.before", { unit: "minute", count: minutes });
}

/** The choices of one reminder: none, and every time offered or held, the earliest first, but the others' own. */
function optionsOf(value: number | undefined, others: readonly number[], t: T): SelectOption[] {
  const times = [...new Set([...offered, ...(value === undefined ? [] : [value])])]
    .filter((m) => !others.includes(m))
    .toSorted((a, b) => b - a);
  return [
    { value: none, label: t("settings.events.none") },
    ...times.map((m) => ({ value: String(m), label: before(m, t) })),
  ];
}

/** Whether the places `slots` hold the times of `minutes`, in any order. */
function sameTimes(slots: readonly (number | undefined)[], minutes: readonly number[]): boolean {
  const held = slots.filter((m) => m !== undefined).toSorted((a, b) => a - b);
  const said = minutes.toSorted((a, b) => a - b);
  return held.length === said.length && held.every((m, i) => m === said[i]);
}

/** The screen of the entry: the reminders, once they are read. */
export function EventSettings({ back }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  const read = useEventReminders();
  const change = useChangeEventReminders();
  const [restoring, setRestoring] = useState<"saving" | "saved" | "failed" | null>(null);
  // The places as the reader arranged them on this screen: the server keeps the reminders the earliest first, and a
  // choice that moved under the reader's pointer to another row once saved would change what they just chose.
  const [arranged, setArranged] = useState<readonly (number | undefined)[] | null>(null);
  const reminders = read.reminders;
  const minutes = reminders?.minutes_before ?? [];
  const slots =
    arranged !== null && sameTimes(arranged, minutes)
      ? arranged
      : Array.from({ length: eventLimits.reminders }, (_, i) => minutes[i]);
  return (
    <SettingsScreen
      icon="events"
      title={t("settings.entry.events")}
      back={back}
      notice={
        reminders === undefined ? (
          <ReadingNotice
            status={read.status === "error" ? "failed" : "loading"}
            retry={read.refetch}
            words={{ loading: t("settings.events.loading"), failed: t("settings.events.failed") }}
          />
        ) : undefined
      }
    >
      {reminders !== undefined && (
        <SettingsSection heading={t("settings.events.reminders")} about={t("settings.events.reminders.about")}>
          {slots.map((value, i) => {
            const others = slots.filter((m, j) => j !== i && m !== undefined) as number[];
            return (
              <Choice
                // A place keeps its row across changes, so that the focus stays on the choice pressed.
                key={i}
                page={eventRemindersPath}
                label={t("settings.events.reminder", { number: i + 1 })}
                options={optionsOf(value, others, t)}
                value={value === undefined ? none : String(value)}
                onChoose={(chosen) => {
                  const next = slots.map((m, j) => (j === i ? (chosen === none ? undefined : Number(chosen)) : m));
                  setArranged(next);
                  return change({ minutesBefore: next.filter((m) => m !== undefined) });
                }}
              />
            );
          })}
          {/* The way back to the defaults stays where it was pressed, the focus on it; what came of it beside it. */}
          <Stack gap="entry-line">
            <Box>
              <Button
                size="timeline"
                touch
                busy={restoring === "saving"}
                onPress={() => {
                  setArranged(null);
                  setRestoring("saving");
                  change({ defaults: true }).then(
                    () => {
                      setRestoring("saved");
                    },
                    () => {
                      setRestoring("failed");
                    },
                  );
                }}
              >
                {t("settings.events.defaults")}
              </Button>
            </Box>
            <Box as="p" role="status">
              {restoring !== null && (
                <SaveState state={restoring}>
                  {restoring === "saved" ? t("settings.events.defaults_done") : undefined}
                </SaveState>
              )}
            </Box>
          </Stack>
        </SettingsSection>
      )}
    </SettingsScreen>
  );
}
