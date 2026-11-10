// SPDX-License-Identifier: AGPL-3.0-only
// The parts that every view of an event shares (ADR-0161 §5, §8): when it is, in the viewer's time zone with the
// event's own beside it when that zone differs ("Sat 9:00 PM – 11:00 PM (Tokyo)"), and its state as badges, each
// with an icon and words (web-client/16-look-states-and-wording.md G10): cancelled, ended, the date still polled,
// and the user's answer or invitation.
import { CheckIcon, CloseIcon, InviteIcon, ScheduleIcon, Stack, StatusBadge, Text } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { Event, EventAnswer } from "../api/events";
import type { EventWhen } from "../hooks/useEventTime";

/** The words of each answer, as a badge of the user's own answer. */
export const answerBadges: Readonly<Record<EventAnswer, ClientMessageKey>> = {
  going: "client:events.answer.going.badge",
  maybe: "client:events.answer.maybe.badge",
  declined: "client:events.answer.declined.badge",
};

/** When the event is: the viewer's time on one line, the event's own time under it when its zone differs. */
export function EventWhenText({ when, size = "base" }: { when: EventWhen; size?: "sm" | "base" }) {
  return (
    <Stack gap="1">
      <Text as="time" dateTime={when.dateTime} size={size} weight="semibold">
        {when.viewer}
      </Text>
      {when.eventZone !== undefined && (
        <Text as="p" size="sm" tone="muted">
          {when.eventZone}
        </Text>
      )}
    </Stack>
  );
}

/** The badges of the event's state and of the user's place in it; nothing for a plain upcoming event. */
export function EventBadges({ event, when }: { event: Event; when: EventWhen }) {
  const t = useT();
  const answer = event.viewer.answer;
  return (
    <>
      {event.state === "cancelled" && (
        <StatusBadge tone="danger" icon={<CloseIcon />} label={t("client:events.state.cancelled")} />
      )}
      {event.state === "scheduled" && when.ended && (
        <StatusBadge tone="neutral" icon={<CheckIcon />} label={t("client:events.state.ended")} />
      )}
      {event.state === "unscheduled" && (
        <StatusBadge tone="warning" icon={<ScheduleIcon />} label={t("client:events.state.unscheduled")} />
      )}
      {answer !== undefined && answer !== "declined" && (
        <StatusBadge tone="success" icon={<CheckIcon />} label={t(answerBadges[answer])} />
      )}
      {answer === undefined && event.viewer.invited && (
        <StatusBadge tone="accent" icon={<InviteIcon />} label={t("client:events.state.invited")} />
      )}
    </>
  );
}
