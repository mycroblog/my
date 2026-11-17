// SPDX-License-Identifier: AGPL-3.0-only

// The poll a post carries, in the look of the composer's poll card (composer.tsx), which mirrors it: a raised card
// whose options sit together on the page's surface inside it, each headed by the mark it is answered with, a radio's
// ring or, where several may be chosen, a checkbox's square. Open to the reader, an option is a pressable row and the
// card ends with its line (who voted, the time left) and Vote. Answered, ended, or the reader's own, the options read as
// results: the reader's choices with the mark knocked out, as an on state is (icons.tsx), and each option's share as a
// figure and a bar across the row, unless its author hides the shares until it ends. Every word is the caller's.

import { useId, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { Box, Layers, Row, Stack } from "./layout";

const poll = tv({
  slots: {
    card: "rounded-control border-0 bg-surface-raised p-embed",
    // The options on the page's surface: a row's own padding lines its mark up with the composer's.
    options: "list-none rounded-control bg-surface p-poll-padding",
    // As tall as a form's control on one line; an option that wraps keeps its mark and its share by its first line.
    row: "box-border rounded-control px-poll-option-inline py-control-form-line text-body text-text",
    choice: [
      "cursor-pointer has-[input:enabled]:hover:bg-surface-hover has-[input:focus-visible]:shadow-focus",
      "has-[input:disabled]:cursor-default",
    ],
    input: "sr-only",
    label: "[overflow-wrap:anywhere]",
    share: "text-body tabular-nums",
    // A result: its line (the mark, the option, its share), then its share as a bar across the whole row, so that the
    // bars of a poll compare true; the bar sits close under the line, one line or several.
    result: "box-border rounded-control px-poll-option-inline py-poll-result-block text-body text-text",
    // The meter's bar (shell.tsx), quieter in a timeline: muted on a track lighter than it.
    track: "h-shell-meter rounded-[calc(var(--shell-meter)/2)] bg-surface-raised",
    bar: "rounded-[calc(var(--shell-meter)/2)] bg-muted-text",
    part: "whitespace-nowrap",
    meta: "text-caption text-muted-text",
  },
  variants: {
    leading: { true: { label: "font-label", share: "font-label" } },
  },
});

const mark = tv({
  base: "box-border size-form-page-check border-[length:var(--composer-mark-line)] border-solid",
  variants: {
    multiple: { false: "rounded-full", true: "rounded-[3px]" },
    on: { false: "", true: "border-text bg-text text-surface" },
    // In results, where nothing is pressed any more: an option not chosen keeps its mark's shape, faint.
    plain: { false: "", true: "" },
  },
  // One border color each (no class merges here): muted to be pressed, faint in results.
  compoundVariants: [
    { on: false, plain: false, class: "border-muted-text" },
    { on: false, plain: true, class: "border-surface-raised-pressed" },
  ],
});

/**
 * The mark an option is answered with: a radio's ring, or a checkbox's square where several may be chosen; on (chosen),
 * knocked out of the text's color, the ring around a dot, the square around a check; `plain` in results, where an
 * option not chosen is no longer to be pressed.
 */
export function PollMark({
  multiple,
  on = false,
  plain = false,
}: {
  multiple: boolean;
  on?: boolean;
  plain?: boolean;
}) {
  return (
    <Layers as="span" center rigid className={mark({ multiple, on, plain })} aria-hidden="true">
      {on &&
        (multiple ? (
          <svg viewBox="0 0 12 12" className="size-poll-mark-check" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M2.5 6.25 L5 8.75 L9.5 3.5" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        ) : (
          <span className="size-poll-mark-dot rounded-full bg-current" />
        ))}
    </Layers>
  );
}

/** The poll: a raised card of its options, then its line and its action. */
export function PollBox({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack shrink gap="entry-line" role="group" aria-label={label} className={poll().card()}>
      {children}
    </Stack>
  );
}

/** The mark's place: as tall as a line, so that the mark stays by the first line of an option that wraps. */
function MarkPlace({ children }: { children: ReactNode }) {
  return (
    <Row as="span" line rigid align="center">
      {children}
    </Row>
  );
}

/** One option open to the reader's vote. */
export type PollChoiceItem = { key: string; label: ReactNode };

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
  const s = poll();
  const name = useId();
  return (
    <Stack role={multiple ? "group" : "radiogroup"} aria-label={label} className={s.options()}>
      {items.map((item, i) => {
        const on = chosen.includes(i);
        return (
          <Row
            key={item.key}
            as="label"
            anchor="isolate"
            align="start"
            gap="entry-caption"
            className={`${s.row()} ${s.choice()}`}
          >
            <input
              className={s.input()}
              type={multiple ? "checkbox" : "radio"}
              name={name}
              checked={on}
              disabled={disabled}
              onChange={() => {
                if (!multiple) onChange([i]);
                else onChange(on ? chosen.filter((c) => c !== i) : [...chosen, i].sort((a, b) => a - b));
              }}
            />
            <MarkPlace>
              <PollMark multiple={multiple} on={on} />
            </MarkPlace>
            <Box as="span" grow className={s.label()} dir="auto">
              {item.label}
            </Box>
          </Row>
        );
      })}
    </Stack>
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
  const s = poll();
  const marked = items.some((item) => item.mine);
  return (
    <Stack as="ul" className={s.options()}>
      {items.map((item) => {
        const r = poll({ leading: item.leading });
        return (
          <Stack key={item.key} as="li" gap="hair" className={s.result()}>
            <Row as="span" align="start" gap="entry-caption">
              {marked && (
                <MarkPlace>
                  <PollMark multiple={multiple} on={item.mine} plain />
                </MarkPlace>
              )}
              <Box as="span" grow className={r.label()} dir="auto">
                {item.label}
                {item.mine && <span className="sr-only">{` (${mineLabel})`}</span>}
              </Box>
              {item.share && (
                <Box as="span" rigid className={r.share()}>
                  {item.share.text}
                </Box>
              )}
            </Row>
            {item.share && (
              <Box as="span" clip className={s.track()} aria-hidden="true">
                <Box as="span" display="block" full="block" share={item.share.value} className={s.bar()} />
              </Box>
            )}
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
  const s = poll();
  return (
    <Row align="center" gap="entry-caption">
      {/* Its line wraps beside Vote, which stays at the end. */}
      <Box as="p" grow className={s.meta()}>
        {parts.map((part, i) => (
          <span key={i}>
            {i > 0 && " · "}
            <span className={s.part()}>{part}</span>
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
