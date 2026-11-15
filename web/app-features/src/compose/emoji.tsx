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
import { useEffect, useState, type RefObject } from "react";
import { usePickerData } from "../emoji/picker";
import { emojiStrings } from "../emoji/strings";
import { stringsIn } from "../locale";

/**
 * `text` with `emoji` written over `start` to `end`: a custom emoji's `:shortcode:` apart from the words beside it by
 * a space where there is none; and where the caret goes after it.
 */
function written(
  text: string,
  start: number,
  end: number,
  emoji: PickerEmoji,
  shown: string,
): { text: string; caret: number } {
  const before = text.slice(0, start);
  const after = text.slice(end);
  const custom = "image" in emoji;
  const lead = custom && before !== "" && !/\s$/u.test(before) ? " " : "";
  const trail = custom && !/^\s/u.test(after) ? " " : "";
  const insert = `${lead}${shown}${trail}`;
  return { text: `${before}${insert}${after}`, caret: start + insert.length };
}

/** After the frame the picker's closing restores focus in, `then` runs. */
const afterFocusReturns = (then: () => void) => {
  requestAnimationFrame(() => {
    requestAnimationFrame(then);
  });
};

/** The emoji button of the text in `textRef`, whose value is `value`; `onChange` takes the text with the emoji. */
export function ComposerEmoji({
  textRef,
  value,
  onChange,
  sheet,
  disabled,
}: {
  textRef: RefObject<HTMLTextAreaElement | null>;
  value: string;
  onChange: (text: string) => void;
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
    const area = textRef.current;
    const start = area?.selectionStart ?? value.length;
    const end = area?.selectionEnd ?? value.length;
    const next = written(value, start, end, emoji, shown);
    onChange(next.text);
    const caret = () => {
      textRef.current?.setSelectionRange(next.caret, next.caret);
    };
    if (keep) {
      afterFocusReturns(caret);
      return;
    }
    setOpen(false);
    afterFocusReturns(() => {
      textRef.current?.focus();
      caret();
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
