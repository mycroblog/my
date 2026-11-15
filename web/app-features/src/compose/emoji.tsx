// SPDX-License-Identifier: AGPL-3.0-only

// The composer's emoji button (ADR-0091 §4, §5): at the end of the text's first line, it opens the same emoji picker
// as a post's react action (emoji/picker.ts), in a popover beside it, or in a sheet on a phone (the app's choice).
// Picking writes the emoji at the caret, or over the selection: a Unicode emoji as it is, in the skin tone chosen, a
// custom one as `:shortcode:` with a space on each side where the text has none, so that the server finds it. The
// picker closes and the text takes the focus back with its caret after what was written; picked with Shift held, the
// picker stays open for the next one and the focus stays on it.

import {
  EmojiPicker,
  EmojiPickerPopover,
  EmojiPickerSheet,
  IconButton,
  preloadEmojiPicker,
  useLocale,
  type PickerEmoji,
} from "@my/design-system";
import type { EditorHandle } from "@my/design-system/editor";
import { useEffect, useState, type RefObject } from "react";
import { usePickerData } from "../emoji/picker";
import { emojiStrings } from "../emoji/strings";
import { stringsIn } from "../locale";

/**
 * What is written for `emoji` between `before` and `after`: a custom emoji's `:shortcode:` apart from the words beside
 * it by a space where there is none.
 */
function written(before: string, after: string, emoji: PickerEmoji, shown: string): string {
  const custom = "image" in emoji;
  const lead = custom && before !== "" && !/\s$/u.test(before) ? " " : "";
  const trail = custom && !/^\s/u.test(after) ? " " : "";
  return `${lead}${shown}${trail}`;
}

/** After the frame the picker's closing restores focus in, `then` runs. */
const afterFocusReturns = (then: () => void) => {
  requestAnimationFrame(() => {
    requestAnimationFrame(then);
  });
};

/** The emoji button of the editor of `text`, which writes the emoji picked at its selection. */
export function ComposerEmoji({
  text,
  sheet,
  disabled,
}: {
  text: RefObject<EditorHandle | null>;
  /** The picker opens in a sheet (a phone's viewport), else in a popover. */
  sheet: boolean;
  disabled: boolean;
}) {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  const { preload, onOpen, ahead, ...data } = usePickerData(locale);
  const [open, setOpen] = useState(false);
  const show = (shown: boolean) => {
    if (shown) onOpen();
    setOpen(shown);
  };
  // The picker's code comes once the composer is idle, and its emoji too for a reader who opened a picker here
  // before; else its emoji come as the reader reaches for the button.
  useEffect(() => ahead(), [ahead]);
  const label = t("compose.emoji");
  // The picker's code and its emoji are fetched as the reader reaches for the button, before it opens.
  const warm = () => {
    preloadEmojiPicker();
    preload();
  };
  const pick = (emoji: PickerEmoji, shown: string, { keep }: { keep: boolean }) => {
    const write = (before: string, after: string) => written(before, after, emoji, shown);
    if (keep) {
      // The focus stays on the picker; the editor keeps its selection after what was written.
      text.current?.insert(write, { focus: false });
      return;
    }
    setOpen(false);
    afterFocusReturns(() => {
      text.current?.insert(write);
    });
  };
  const content = <EmojiPicker {...data} fill={sheet} autoFocus onSelect={pick} />;
  const button = (
    <IconButton
      icon="emojiSmileys"
      label={label}
      isDisabled={disabled}
      onHoverStart={warm}
      onFocus={warm}
      onPressStart={warm}
      {...(sheet
        ? {
            onPress: () => {
              show(true);
            },
          }
        : {})}
    />
  );
  if (sheet)
    return (
      <>
        {button}
        <EmojiPickerSheet
          open={open}
          onClose={() => {
            setOpen(false);
          }}
          title={label}
          closeLabel={emojiStrings(locale, []).sheet.close}
        >
          {content}
        </EmojiPickerSheet>
      </>
    );
  return (
    <EmojiPickerPopover label={label} trigger={button} open={open} onOpenChange={show} placement="bottom end">
      {content}
    </EmojiPickerPopover>
  );
}
