// SPDX-License-Identifier: AGPL-3.0-only
// The emoji that a voice chat room shows before its name (FR-923): a Unicode emoji, or a custom emoji of the server
// drawn as its image. It only decorates: the name beside it says what the room is, so assistive technology skips it
// and reads the name, which it never replaces. It is a word of the name's line, as high as its text.
import { tv } from "tailwind-variants";
import { CustomEmoji, type CustomEmojiImage } from "./CustomEmoji";

const voiceRoomIcon = tv({ base: "me-[0.3em] inline-block" });

/** The emoji of a room's icon: a Unicode emoji, or a custom emoji with its image. */
export type VoiceRoomIconEmoji = { unicode: string } | { custom: CustomEmojiImage };

export type VoiceRoomIconProps = {
  emoji: VoiceRoomIconEmoji;
};

/** The icon before a room's name: put it right before the name, inside the same link or heading. */
export function VoiceRoomIcon({ emoji }: VoiceRoomIconProps) {
  return (
    <span aria-hidden="true" data-voice-room-icon="" className={voiceRoomIcon()}>
      {"unicode" in emoji ? emoji.unicode : <CustomEmoji emoji={emoji.custom} />}
    </span>
  );
}
