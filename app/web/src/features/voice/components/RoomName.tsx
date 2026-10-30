// SPDX-License-Identifier: AGPL-3.0-only
// A room's name with its emoji icon before it (FR-923), wherever the name shows: the cards of the lists, the stage's
// heading, the mini-player and the summary. The icon only decorates (VoiceRoomIcon): the name says what the room is,
// and a room without an icon, or whose custom emoji was disabled or deleted, shows its name alone.
import { VoiceRoomIcon } from "@my/ui";
import type { VoiceRoomEmoji } from "../api/voice";

/** The icon of a room, or nothing without one. */
export function RoomIcon({ icon }: { icon: VoiceRoomEmoji | undefined }) {
  if (icon === undefined) return null;
  return <VoiceRoomIcon emoji={icon.emoji === undefined ? { unicode: icon.name } : { custom: icon.emoji }} />;
}

/** A room's icon and its name, the host's text in any language: put it where the name goes. */
export function RoomName({ name, icon }: { name: string; icon: VoiceRoomEmoji | undefined }) {
  return (
    <>
      <RoomIcon icon={icon} />
      <span dir="auto">{name}</span>
    </>
  );
}
