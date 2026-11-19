// SPDX-License-Identifier: AGPL-3.0-only

// The controls of a choice among options: the options as pressable rows on the page's surface, each headed by the
// mark it is answered with, a radio's ring or, where several may be chosen, a checkbox's square; the mark alone, for
// what shows a choice made or to be made without being pressed; an option as it is written, a field headed by that
// mark; and a share of a whole as a bar. Every word is the caller's.

import { useId, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { FieldError } from "./form-page";
import { IconButton } from "./icon-button";
import { Box, Layers, Pin, Row, Stack } from "./layout";

const choice = tv({
  slots: {
    // The options on the page's surface: a row's own padding lines its mark up with the composer's.
    options: "list-none rounded-control bg-surface p-poll-padding",
    // As tall as a form's control on one line; an option that wraps keeps its mark by its first line.
    row: [
      "box-border rounded-control px-poll-option-inline py-control-form-line text-body text-text",
      "cursor-pointer has-[input:enabled]:hover:bg-surface-hover has-[input:focus-visible]:shadow-focus",
      "has-[input:disabled]:cursor-default",
    ],
    // Over the whole of its row and not drawn: a press or a click anywhere on the option lands on the control itself.
    input: "cursor-pointer opacity-0 disabled:cursor-default",
    label: "[overflow-wrap:anywhere]",
    // An option as it is written: its text on the surface it stands on, the ring around it on focus.
    field: [
      "h-control-form rounded-control border-0 bg-transparent p-0 font-sans text-body text-text",
      "outline-hidden placeholder:text-muted-text focus-visible:shadow-focus",
    ],
    // The meter's bar (shell.tsx), quieter in a timeline: muted on a track lighter than it.
    track: "h-shell-meter rounded-[calc(var(--shell-meter)/2)] bg-surface-raised",
    bar: "rounded-[calc(var(--shell-meter)/2)] bg-muted-text",
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
 * knocked out of the text's color, the ring around a dot, the square around a check; `plain` where nothing is pressed
 * any more (results), and an option not chosen keeps its mark's shape, faint.
 */
export function ChoiceMark({
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

/** The mark's place: as tall as a line, so that the mark stays by the first line of an option that wraps. */
function MarkPlace({ children }: { children: ReactNode }) {
  return (
    <Row as="span" line rigid align="center">
      {children}
    </Row>
  );
}

/** One option open to a choice. */
export type ChoiceItem = { key: string; label: ReactNode };

/**
 * The options open to a choice, named `label`: a radio or a checkbox each, as a pressable row headed by its mark;
 * `chosen` are the indexes chosen so far, and `disabled` holds them (an answer in flight). With `id`, the choice as a
 * whole takes the focus when what holds it says it is refused (`focusRefusedField`), its error among `describedBy`.
 */
export function ChoiceList({
  label,
  multiple,
  items,
  chosen,
  onChange,
  disabled = false,
  id,
  describedBy,
}: {
  label: string;
  multiple: boolean;
  items: readonly ChoiceItem[];
  chosen: readonly number[];
  onChange: (chosen: number[]) => void;
  disabled?: boolean;
  id?: string | undefined;
  describedBy?: string | undefined;
}) {
  const s = choice();
  const name = useId();
  return (
    <Stack
      id={id}
      tabIndex={id === undefined ? undefined : -1}
      role={multiple ? "group" : "radiogroup"}
      aria-label={label}
      aria-describedby={describedBy}
      className={s.options()}
    >
      {items.map((item, i) => {
        const on = chosen.includes(i);
        return (
          <Row key={item.key} as="label" anchor="isolate" align="start" gap="entry-caption" className={s.row()}>
            <Pin
              cover
              full="both"
              as={
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
              }
            />
            <MarkPlace>
              <ChoiceMark multiple={multiple} on={on} />
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

/**
 * An option as it is written: a field named by `label`, headed by the mark it will be answered with (a checkbox's
 * square where several may be chosen, else a radio's ring); its remove at its end, where it may go, and what is wrong
 * with it right under it.
 */
export function ChoiceField({
  value,
  onChange,
  label,
  multiple,
  maxLength,
  remove,
  error,
}: {
  value: string;
  onChange: (value: string) => void;
  label: string;
  multiple: boolean;
  maxLength: number;
  remove?: { label: string; onPress: () => void } | undefined;
  error?: string | undefined;
}) {
  const s = choice();
  const errorId = useId();
  return (
    <>
      <Row align="center" gap="entry-caption">
        <ChoiceMark multiple={multiple} />
        <Box
          as={
            <input
              value={value}
              placeholder={label}
              aria-label={label}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? errorId : undefined}
              maxLength={maxLength}
              dir="auto"
              onChange={(e) => {
                onChange(e.target.value);
              }}
            />
          }
          grow
          className={s.field()}
        />
        {remove && <IconButton icon="close" label={remove.label} onPress={remove.onPress} />}
      </Row>
      {error && <FieldError id={errorId}>{error}</FieldError>}
    </>
  );
}

/** A share of a whole (0 to 1) as a bar across what holds it, so that the bars of a list compare true. */
export function ShareBar({ share }: { share: number }) {
  const s = choice();
  return (
    <Box as="span" display="block" clip className={s.track()} aria-hidden="true">
      <Box as="span" display="block" full="block" share={share} className={s.bar()} />
    </Box>
  );
}
