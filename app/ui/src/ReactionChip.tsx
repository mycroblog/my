// SPDX-License-Identifier: AGPL-3.0-only
// The emoji reactions under a post (FR-116, ADR-0052 §5): a row of chips, one per emoji with its count. A chip is a toggle
// the viewer presses to react with that emoji or to take the reaction back, or, for a custom emoji of another server that
// nobody here can react with, the same shown only. The row ends with a button that lists the accounts that reacted.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { Cluster } from "./Cluster";
import { CustomEmoji, type CustomEmojiImage } from "./CustomEmoji";
import { Text } from "./Text";
import { ToggleButton } from "./ToggleButton";

const chip = tv({
  base: "rounded-full bg-surface px-3 font-bold text-fg data-selected:bg-soft data-selected:text-accent",
});

const staticChip = tv({
  base: "inline-flex min-h-8 items-center gap-1.5 rounded-full border border-border bg-surface px-3 text-sm font-bold text-fg-muted",
});

export type ReactionEmojiProps = {
  /** The image of a custom emoji; without it the emoji is text. */
  image?: CustomEmojiImage | undefined;
  /** The Unicode emoji, or the `:shortcode:` of a custom emoji that has no image. */
  text: string;
};

/** The emoji of a reaction: the image of a custom emoji, or the text of a Unicode emoji. */
export function ReactionEmoji({ image, text }: ReactionEmojiProps) {
  if (image !== undefined) return <CustomEmoji emoji={image} />;
  return (
    <Text size="base" dir="auto">
      {text}
    </Text>
  );
}

export type ReactionChipProps = ReactionEmojiProps & {
  /** The count, already formatted for the locale. */
  count: string;
  /**
   * The accessible name of the toggle, which starts with the emoji and the count as they are shown ("👍 3, react with
   * this emoji"; WCAG 2.5.3); a chip that is no toggle is read as its emoji and count.
   */
  label: string;
  /** Whether the viewer has reacted with this emoji. */
  isSelected: boolean;
  /** Toggles the viewer's reaction; `null` shows the chip only, for an emoji nobody here can react with. */
  onChange: ((isSelected: boolean) => void) | null;
};

/** One emoji with its count: a toggle named by `label`, or the same shown only. */
export function ReactionChip({ image, text, count, label, isSelected, onChange }: ReactionChipProps) {
  const countText = <Text numeric>{count}</Text>;
  if (onChange === null) {
    return (
      <span className={staticChip()}>
        <ReactionEmoji image={image} text={text} />
        {countText}
      </span>
    );
  }
  return (
    <ToggleButton
      aria-label={label}
      tone="accent"
      isSelected={isSelected}
      onChange={onChange}
      outlined
      className={chip()}
    >
      <ReactionEmoji image={image} text={text} />
      {countText}
    </ToggleButton>
  );
}

export type ReactionRowProps = {
  /** The accessible name of the group. */
  label: string;
  /** The text of the button that lists who reacted. */
  listLabel: string;
  onOpenList: () => void;
  /** The `ReactionChip`s. */
  children: ReactNode;
  /** Rendered after the button: the dialog the button opens. */
  after?: ReactNode;
};

/** The reactions of a post in a wrapping group, ending with the button that lists who reacted. */
export function ReactionRow({ label, listLabel, onOpenList, children, after }: ReactionRowProps) {
  return (
    <Cluster role="group" aria-label={label} gap="1" align="center" shrink>
      {children}
      <Button variant="subtle" size="sm" onPress={onOpenList}>
        {listLabel}
      </Button>
      {after}
    </Cluster>
  );
}
