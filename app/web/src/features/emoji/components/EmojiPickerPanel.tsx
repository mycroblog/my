// SPDX-License-Identifier: AGPL-3.0-only
import { EmojiPicker, type EmojiPickerSelection, InlineError } from "@my/ui";
import { useT } from "../../../lib/i18n";
import type { EmojiPickerContent } from "../hooks/emojiPickerContent";

export type EmojiPickerPanelProps = {
  picker: Omit<EmojiPickerContent, "remember">;
  onSelect: (selection: EmojiPickerSelection) => void;
};

/**
 * The inside of the popover of an emoji picker (ADR-0091 §4): the picker with its search field focused at once, with
 * skeleton rows in its grid until the Unicode emoji arrive (web-client/16-look-states-and-wording.md G15), under an
 * error line when they could not be loaded (the custom emoji are still offered).
 */
export function EmojiPickerPanel({ picker, onSelect }: EmojiPickerPanelProps) {
  const t = useT();
  return (
    <>
      {picker.status === "error" && <InlineError>{t("client:emoji.picker.load_failed")}</InlineError>}
      <EmojiPicker
        sections={picker.sections}
        loading={picker.status === "loading"}
        labels={picker.labels}
        skinTone={picker.skinTone}
        onSkinToneChange={picker.setSkinTone}
        autoFocus
        onSelect={onSelect}
      />
    </>
  );
}
