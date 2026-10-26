// SPDX-License-Identifier: AGPL-3.0-only
// The entry to the open room of a profile (ADR-0100 §8, 回答 33): while the account hosts a room or speaks in it and
// the user sees the room, a note that says so with the room's name as a link to its page. Being a listener is
// never shown, and nothing shows while voice chat is off.
import { Box, Callout, Link } from "@my/ui";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { voiceRoomPath } from "../api/voice";
import { useProfileVoiceRoom } from "../hooks/useVoiceHistory";

export type ProfileVoiceRoomProps = {
  accountId: string;
  /** The account's name, isolated for a sentence (`⁨name⁩`). */
  name: string;
};

export function ProfileVoiceRoom({ accountId, name }: ProfileVoiceRoomProps) {
  const t = useT();
  const intl = useIntl();
  const room = useProfileVoiceRoom(accountId);
  if (room === undefined) return null;
  const link = (
    <Link key="room" href={voiceRoomPath(room.id)} variant="standalone">
      <span dir="auto">{room.name}</span>
    </Link>
  );
  return (
    // Inset like the profile's other notes under its header.
    <Box paddingX="4">
      <Callout>
        <p>{intl.formatMessage({ id: room.role === "host" ? "client:voice.profile.host" : "client:voice.profile.speaker" }, { name, room: link })}</p>
        <p>{t("client:voice.room.participants_count", { count: room.participants_count })}</p>
      </Callout>
    </Box>
  );
}
