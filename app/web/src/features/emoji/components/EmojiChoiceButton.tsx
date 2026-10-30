// SPDX-License-Identifier: AGPL-3.0-only
import { Button, Popover, PopoverTrigger } from "@my/ui";
import type { ReactNode } from "react";
import { useT } from "../../../lib/i18n";
import { type ReactionChoice, useReactionPicker } from "../hooks/useReactionPicker";
import { EmojiPickerPanel } from "./EmojiPickerPanel";

export type EmojiChoiceButtonProps = {
  /** What the button shows and is named by: words that say what the emoji is for ("Choose an icon"), with the emoji chosen now. */
  children: ReactNode;
  /** Called with the emoji picked; the popover is closed by then. */
  onSelect: (choice: ReactionChoice) => void;
  isDisabled?: boolean | undefined;
};

/**
 * A button that opens the post form's emoji picker in a popover (WAI-ARIA APG, dialog pattern; ADR-0091 §4) to choose
 * one emoji, such as the icon of a voice chat room (FR-923): the emoji used often, the custom emoji of this server and
 * every Unicode emoji in the skin tone chosen here, the same emoji that a reaction takes (ADR-0052 §2). Hovering or
 * focusing it starts loading the Unicode emoji; the popover opens at once with its search field focused. Picking an
 * emoji closes the popover, which returns focus to the button. The button is named by the words it shows.
 */
export function EmojiChoiceButton({ children, onSelect, isDisabled = false }: EmojiChoiceButtonProps) {
  const t = useT();
  const picker = useReactionPicker(t("client:emoji.picker.grid"));
  return (
    <PopoverTrigger
      onOpenChange={(isOpen) => {
        if (isOpen) picker.preload();
      }}
    >
      <Button isDisabled={isDisabled} onHoverStart={picker.preload} onFocus={picker.preload}>
        {children}
      </Button>
      <Popover aria-label={t("client:emoji.picker.title")} placement="bottom start">
        {({ close }) => (
          <EmojiPickerPanel
            picker={picker}
            onSelect={(selection) => {
              picker.select(selection, close, onSelect);
            }}
          />
        )}
      </Popover>
    </PopoverTrigger>
  );
}
