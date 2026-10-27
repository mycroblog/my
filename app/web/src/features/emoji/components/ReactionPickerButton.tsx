// SPDX-License-Identifier: AGPL-3.0-only
import { Button, CallDockButton, EmojiIcon, Popover, PopoverTrigger, Tooltip, TooltipTrigger } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { type ReactionChoice, useReactionPicker } from "../hooks/useReactionPicker";
import { EmojiPickerPanel } from "./EmojiPickerPanel";

export type ReactionPickerButtonProps = {
  /** Called with the emoji picked; the popover is closed by then. */
  onSelect: (choice: ReactionChoice) => void;
  isDisabled?: boolean | undefined;
  /** `toolbar` (default): an icon button of a post's action bar; `dock`: a control of a voice chat call's dock, with the popover above it. */
  appearance?: "toolbar" | "dock";
};

/**
 * An icon button of a post's action bar (or of a call's dock) that opens the picker of reactions in a popover
 * (WAI-ARIA APG, dialog pattern): the post form's emoji picker (ADR-0091 §7), with the emoji used often, the custom
 * emoji of this server and every Unicode emoji in the skin tone chosen here. Hovering or focusing it (in the dock,
 * opening it) starts loading the Unicode emoji, and the popover shows a spinner until they arrive. The search field
 * takes focus; picking an emoji closes the popover, which returns focus to the button, and reacts with it, also when
 * Shift is held (one reaction per post, ADR-0052 §1). The button is named by aria-label and shows the same words as a
 * tooltip (web-client/16-look-states-and-wording.md G12).
 */
export function ReactionPickerButton({ onSelect, isDisabled = false, appearance = "toolbar" }: ReactionPickerButtonProps) {
  const t = useT();
  const picker = useReactionPicker();
  const name = t("client:reaction.picker.button");
  const trigger = (
    <PopoverTrigger
      onOpenChange={(isOpen) => {
        if (isOpen) picker.preload();
      }}
    >
      {appearance === "dock" ? (
        // The dock's control shows its own tooltip.
        <CallDockButton label={name} icon={<EmojiIcon />} isDisabled={isDisabled} />
      ) : (
        <Button variant="subtle" size="toolbar" aria-label={name} isDisabled={isDisabled} onHoverStart={picker.preload} onFocus={picker.preload}>
          <EmojiIcon />
        </Button>
      )}
      <Popover aria-label={t("client:reaction.picker.title")} placement={appearance === "dock" ? "top" : "bottom start"}>
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
  if (appearance === "dock") return trigger;
  return (
    <TooltipTrigger>
      {trigger}
      <Tooltip>{name}</Tooltip>
    </TooltipTrigger>
  );
}
