// SPDX-License-Identifier: AGPL-3.0-only

// The poll a post carries, in the look of the composer's poll card, which mirrors it: a raised card whose options sit
// together on the page's surface inside it, each headed by the mark it is answered with. Open to the reader, an option
// is a pressable row (the design system's choices) and the card ends with its line (who voted, the time left) and
// Vote. Answered, ended, or the reader's own, the options read as results: the reader's choices with the mark knocked
// out, and each option's share as a figure and a bar across the row, unless its author hides the shares until it
// ends. Every word is the caller's. It is composed of the design system's parts alone, laid out and painted by their
// props (ADR-0025 §4).

import { Box, Button, ChoiceList, ChoiceMark, Row, ShareBar, Spoken, Stack, type ChoiceItem } from "@my/design-system";
import type { ReactNode } from "react";

/** The poll: a raised card of its options, then its line and its action. */
export function PollBox({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack shrink gap="entry-line" surface="raised" radius="control" pad="embed" role="group" aria-label={label}>
      {children}
    </Stack>
  );
}

/** One option open to the reader's vote. */
export type PollChoiceItem = ChoiceItem;

/**
 * The options open to the reader: a radio or a checkbox each, as a pressable row headed by its mark; `chosen` are the
 * indexes chosen so far, and `disabled` holds them (a vote in flight).
 */
export function PollChoices({
  label,
  multiple,
  items,
  chosen,
  onChange,
  disabled = false,
}: {
  label: string;
  multiple: boolean;
  items: readonly PollChoiceItem[];
  chosen: readonly number[];
  onChange: (chosen: number[]) => void;
  disabled?: boolean;
}) {
  return (
    <ChoiceList
      label={label}
      multiple={multiple}
      items={items}
      chosen={chosen}
      onChange={onChange}
      disabled={disabled}
    />
  );
}

/** One option as a result: its share of the votes (0 to 1) and that share as text, unless hidden until it ends. */
export type PollResultItem = {
  key: string;
  label: ReactNode;
  share?: { value: number; text: string } | undefined;
  /** The reader chose it. */
  mine: boolean;
  /** It has the most votes, once the poll has ended. */
  leading: boolean;
};

/**
 * The options as results; `mineLabel` names the reader's choice for assistive technology. Each is a line of its mark,
 * the option and its share, over its share as a bar. Where the reader chose, the options keep their marks, the chosen
 * knocked out and the others plain; where they did not (ended, their own), no mark stands for a choice.
 */
export function PollResults({
  multiple,
  items,
  mineLabel,
}: {
  multiple: boolean;
  items: readonly PollResultItem[];
  mineLabel: string;
}) {
  const marked = items.some((item) => item.mine);
  return (
    // The options on the page's surface, as the choices are.
    <Stack as="ul" plain surface="page" radius="control" pad="poll-padding">
      {items.map((item) => {
        const weight = item.leading ? "label" : undefined;
        return (
          // A result: its line (the mark, the option, its share), then its share as a bar across the whole row; the
          // bar sits close under the line, one line or several.
          <Stack
            key={item.key}
            as="li"
            gap="hair"
            radius="control"
            padX="poll-option-inline"
            padY="poll-result-block"
            text="body"
            tone="text"
          >
            <Row as="span" align="start" gap="entry-caption">
              {marked && (
                // As tall as a line, so that the mark stays by the first line of an option that wraps.
                <Row as="span" line rigid align="center">
                  <ChoiceMark multiple={multiple} on={item.mine} plain />
                </Row>
              )}
              <Box as="span" grow breaks="anywhere" weight={weight} dir="auto">
                {item.label}
                {item.mine && <Spoken>{` (${mineLabel})`}</Spoken>}
              </Box>
              {item.share && (
                <Box as="span" rigid text="body" numeric weight={weight}>
                  {item.share.text}
                </Box>
              )}
            </Row>
            {item.share && <ShareBar share={item.share.value} />}
          </Stack>
        );
      })}
    </Stack>
  );
}

/**
 * The poll's last line: its parts (that several may be chosen, who voted, the time left, that its shares show when it
 * ends), each kept whole, and the vote, which waits for a choice and spins while it is in flight.
 */
export function PollFoot({
  parts,
  vote,
}: {
  parts: readonly string[];
  vote?: { label: string; disabled: boolean; busy: boolean; onPress: () => void } | undefined;
}) {
  return (
    <Row align="center" gap="entry-caption">
      {/* Its line wraps beside Vote, which stays at the end. */}
      <Box as="p" grow text="caption" tone="muted">
        {parts.map((part, i) => (
          <span key={i}>
            {i > 0 && " · "}
            <Box as="span" nowrap>
              {part}
            </Box>
          </span>
        ))}
      </Box>
      {vote && (
        <Button kind="primary" size="timeline" isDisabled={vote.disabled} busy={vote.busy} onPress={vote.onPress}>
          {vote.label}
        </Button>
      )}
    </Row>
  );
}
