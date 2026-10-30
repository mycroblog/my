// SPDX-License-Identifier: AGPL-3.0-only
// The picker of the emoji to react to a post with (ADR-0091 §7; ADR-0052 §1, §5): the same emoji as the post form's
// picker (the emoji used often, the custom emoji of this server, every Unicode emoji in the skin tone chosen here).
// The Unicode data comes from the server's EMOJI_VERSION, so the server accepts every emoji it offers; the custom emoji
// of other servers are not offered. A post takes one reaction, so picking one always closes the picker.
import type { EmojiPickerSelection, PickerCustomEmoji } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { type EmojiPickerContent, useEmojiPickerContent } from "./emojiPickerContent";

/** The emoji that a reaction is made with: a Unicode emoji (in its skin tone), or a custom emoji of this server. */
export type ReactionChoice = { unicode: string } | { custom: PickerCustomEmoji };

export type ReactionPickerControl = Omit<EmojiPickerContent, "remember"> & {
  /**
   * Closes the picker with `close`, counts the emoji of `selection` as used and reacts with it through `react`, also
   * when the selection asks to keep the picker open (one reaction per post, ADR-0052 §1).
   */
  select: (selection: EmojiPickerSelection, close: () => void, react: (choice: ReactionChoice) => void) => void;
};

/**
 * The emoji of the picker of reactions, or of another choice of one emoji that a reaction could take (the icon of a
 * voice chat room, FR-923), whose grid `gridLabel` names.
 */
export function useReactionPicker(gridLabel?: string): ReactionPickerControl {
  const t = useT();
  const { remember, ...content } = useEmojiPickerContent(gridLabel ?? t("client:reaction.picker.grid"));
  return {
    ...content,
    select: (selection, close, react) => {
      close();
      remember(selection);
      react(selection.kind === "custom" ? { custom: selection.emoji } : { unicode: selection.text });
    },
  };
}
