// SPDX-License-Identifier: AGPL-3.0-only
// The voice chat place of an event (ADR-0161 §6, §10): the room of this server that opens at the start, with who may
// speak in it without asking. Before the start it says when the room opens; from the start, the way into the room
// (its page, where the user joins the call), or that it is still opening while the page asks again
// (useEventPage), or why it could not open. Everyone who sees the event sees this place: the room's own visibility
// guards who joins.
import { ButtonLink, Stack, Text } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { speakerLabels, voiceRoomPath } from "../../voice";
import type { Event, EventPlace, EventRoomUnavailableReason } from "../api/events";
import type { EventWhen } from "../hooks/useEventTime";

/** Why the room could not open, in the user's words. */
const unavailableReasons: Readonly<Record<EventRoomUnavailableReason, ClientMessageKey>> = {
  disabled: "client:events.place.voice.unavailable.disabled",
  not_eligible: "client:events.place.voice.unavailable.not_eligible",
  too_many_rooms: "client:events.place.voice.unavailable.too_many_rooms",
  hosting_elsewhere: "client:events.place.voice.unavailable.hosting_elsewhere",
};

export function VoicePlace({ place, event, when }: { place: EventPlace; event: Event; when: EventWhen }) {
  const t = useT();
  return (
    <Stack gap="2">
      <Text as="p" size="sm" tone="muted">
        {t("client:events.place.voice")}
      </Text>
      <VoiceRoomState place={place} event={event} when={when} />
      <Text as="p" size="sm" tone="muted">
        {t("client:events.place.voice.speakers", { speakers: t(speakerLabels[place.speakers ?? "manual"]) })}
      </Text>
    </Stack>
  );
}

function VoiceRoomState({ place, event, when }: { place: EventPlace; event: Event; when: EventWhen }) {
  const t = useT();
  if (event.state === "cancelled") return <Text as="p">{t("client:events.place.voice.cancelled")}</Text>;
  if (place.room_unavailable !== undefined) {
    return <Text as="p">{t(unavailableReasons[place.room_unavailable])}</Text>;
  }
  if (place.room_id !== undefined) {
    return (
      <Stack gap="2">
        <Text as="p">{t("client:events.place.voice.open")}</Text>
        <div>
          <ButtonLink href={voiceRoomPath(place.room_id)} variant="primary">
            {t("client:events.place.voice.join")}
          </ButtonLink>
        </div>
      </Stack>
    );
  }
  if (event.state === "scheduled" && when.started) {
    return (
      <Text as="p" role="status">
        {t("client:events.place.voice.opening")}
      </Text>
    );
  }
  return <Text as="p">{t("client:events.place.voice.description")}</Text>;
}
