// SPDX-License-Identifier: AGPL-3.0-only
// What the voice chat pages show while their first data loads (web-client/16-look-states-and-wording.md G15): a busy
// region in the shape of what comes (the rooms' cards, the history's cards, a room's stage), hidden from assistive
// technology, named once by a visually hidden text. The routes show them while their loader waits, and the views while
// their query does.
import { FactsCardSkeleton, Stack, StageSkeleton, VisuallyHidden, VoiceRoomCardSkeleton } from "@my/ui";
import { useT } from "../../../lib/i18n";

/** Placeholder cards, for as long as the first page of a list loads. */
const placeholders = ["first", "second", "third"] as const;

/** The list of open rooms (`/voice`) while its first page loads. */
export function VoiceRoomsSkeleton() {
  const t = useT();
  return (
    <Stack gap="3" aria-busy="true">
      <VisuallyHidden>{t("client:voice.list.loading")}</VisuallyHidden>
      {placeholders.map((key) => (
        <VoiceRoomCardSkeleton key={key} />
      ))}
    </Stack>
  );
}

/** The history of the rooms the user hosted (`/settings/voice`) while its first page loads. */
export function VoiceHistorySkeleton() {
  const t = useT();
  return (
    <Stack gap="3" aria-busy="true">
      <VisuallyHidden>{t("client:voice.list.loading")}</VisuallyHidden>
      {placeholders.map((key) => (
        <FactsCardSkeleton key={key} />
      ))}
    </Stack>
  );
}

/** A room's page (`/voice/{id}`) while the room loads: the shape of its stage. */
export function VoiceStageSkeleton() {
  const t = useT();
  return (
    <div aria-busy="true">
      <VisuallyHidden>{t("client:voice.room.loading")}</VisuallyHidden>
      <StageSkeleton />
    </div>
  );
}
