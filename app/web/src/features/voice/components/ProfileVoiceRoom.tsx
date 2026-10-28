// SPDX-License-Identifier: AGPL-3.0-only
// The entry to the open room of a profile (ADR-0100 §8, 回答 33): while the account hosts a room or speaks in it and
// the user sees the room, the room's card in the same look as in the lists of rooms (VoiceRoomCard): the "Live" pill
// with how many are in it ("Joined" for the room of the user's call), the room's name as a link to its page, and the
// account with its role. Being a listener is never shown, and nothing shows while voice chat is off.
import { Box, Stack, VoiceRoomCard } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { voiceRoomPath } from "../api/voice";
import { useJoinedVoiceRoomId } from "../hooks/useVoiceCallControls";
import { useProfileVoiceRoom } from "../hooks/useVoiceHistory";

export type ProfileVoiceRoomProps = {
  accountId: string;
  /** The account's name. */
  name: string;
};

export function ProfileVoiceRoom({ accountId, name }: ProfileVoiceRoomProps) {
  const t = useT();
  const room = useProfileVoiceRoom(accountId);
  const joinedRoomId = useJoinedVoiceRoomId();
  if (room === undefined) return null;
  const count = t("client:voice.room.participants_count", { count: room.participants_count });
  return (
    // Inset like the profile's other notes under its header.
    <Box paddingX="4">
      <Stack as="ul" aria-label={t("client:voice.rail.title")}>
        <VoiceRoomCard
          name={room.name}
          href={voiceRoomPath(room.id)}
          headingLevel={2}
          liveLabel={t("client:voice.room.live")}
          stats={count}
          joinedLabel={joinedRoomId === room.id ? t("client:voice.room.joined_badge") : undefined}
          speakers={[]}
          host={<span dir="auto">{name}</span>}
          hostLabel={room.role === "host" ? t("client:voice.role.host") : t("client:voice.role.speaker")}
        />
      </Stack>
    </Box>
  );
}
