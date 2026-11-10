// SPDX-License-Identifier: AGPL-3.0-only
import { Box, Button, Hint, InlineError, Popover, PopoverTrigger, Stack, TextField } from "@my/ui";
import { type ReactNode, type SubmitEvent, useState } from "react";
import { useT } from "../../../lib/i18n";
import { useSchedule } from "../hooks/useSchedule";

export type SchedulePickerProps = {
  /** The button that opens the popover (a `Button` of `@my/ui`), given whether the popover is open (its tooltip stays closed then). */
  trigger: (isOpen: boolean) => ReactNode;
  /** When the draft is scheduled now (an ISO date-time), which the fields start from; null starts at the next full hour. */
  scheduledAt: string | null;
  /** Why the draft cannot be scheduled at all (localized), checked when "Schedule" is pressed; null when it can. */
  check?: () => string | null;
  /** Schedules the draft for `at`: resolves to null once it is scheduled, or to the localized reason it is not. */
  onSchedule: (at: Date) => Promise<string | null>;
  /** Called once the popover has closed after a schedule, to say what happened. */
  onScheduled?: () => void;
};

/**
 * When to publish a draft (web-client/screens/compose.md §4, drafts.md §4): a popover (WAI-ARIA APG dialog pattern,
 * non-modal) with a date field and a time field in the user's time zone, which it names, and "Schedule". A time
 * outside the instance's limits shows as the fields' error and is not sent; a refusal of the server shows in the
 * popover, which stays open. The fields are native date and time inputs, which every browser offers a picker for.
 */
export function SchedulePicker({ trigger, scheduledAt, check, onSchedule, onScheduled }: SchedulePickerProps) {
  const t = useT();
  const [isOpen, setIsOpen] = useState(false);
  return (
    <PopoverTrigger isOpen={isOpen} onOpenChange={setIsOpen}>
      {trigger(isOpen)}
      <Popover title={t("client:drafts.schedule.label")} placement="bottom start">
        {({ close }) => (
          <ScheduleFields
            scheduledAt={scheduledAt}
            check={check}
            onSchedule={onSchedule}
            onDone={() => {
              close();
              onScheduled?.();
            }}
          />
        )}
      </Popover>
    </PopoverTrigger>
  );
}

type ScheduleFieldsProps = {
  scheduledAt: string | null;
  check: (() => string | null) | undefined;
  onSchedule: (at: Date) => Promise<string | null>;
  onDone: () => void;
};

/** The popover's content: it mounts when the popover opens, so the fields start afresh each time. */
function ScheduleFields({ scheduledAt, check, onSchedule, onDone }: ScheduleFieldsProps) {
  const t = useT();
  const fields = useSchedule({ initial: scheduledAt, ...(check !== undefined && { check }), schedule: onSchedule });
  // The popover renders in a portal, but React's events go up the component tree: the submit stops here, so that it
  // never reaches the new-post form around the trigger.
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    event.stopPropagation();
    fields.submit(onDone);
  };
  return (
    <Stack as="form" gap="3" onSubmit={onSubmit}>
      <Hint>{t("client:drafts.schedule.time_zone", { zone: fields.zoneName })}</Hint>
      <TextField
        type="date"
        label={t("client:drafts.schedule.date")}
        value={fields.date}
        onChange={fields.setDate}
        isRequired
        autoFocus
      />
      <TextField
        type="time"
        label={t("client:drafts.schedule.time")}
        value={fields.time}
        onChange={fields.setTime}
        isRequired
      />
      {fields.error !== null && <InlineError size="inherit">{fields.error}</InlineError>}
      <Box self="end">
        <Button type="submit" size="sm" variant="primary" isPending={fields.isPending}>
          {t("client:drafts.schedule.submit")}
        </Button>
      </Box>
    </Stack>
  );
}
