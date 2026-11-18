// SPDX-License-Identifier: AGPL-3.0-only

// Two fields a setting needs beside the text field and the switch. A choice from a long list (a language, a time
// zone) is the platform's own picker: on a phone the system's list, which a finger scrolls and searches as everywhere
// else on the device; on a desktop a list the keyboard jumps in by typing a name; nothing of ours to load, to
// translate or to keep accessible, and a list of hundreds costs no more than one of three. Closed, it is drawn as a
// field as wide as the value it holds, a chevron at its end, with the select itself lying over it unseen, so that a
// press anywhere on it opens the picker: a select of its own is as wide as its longest option, which breaks a row its
// value would fit and leaves the fields of a screen ragged. It fills the room it is given (a line of its own under its
// label), its value cut where it does not fit. A number (days, a threshold) is a short field whose figures line up at
// its end, with its unit after it. Both are a control's height, on the neutral surface of a control, and on a raised
// surface (a section of a settings screen) take the page's, as every field does.

import { useState, type ReactElement, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Icon } from "./icons";
import { Box, Row } from "./layout";

const field = tv({
  slots: {
    // The field the reader sees: the value and the chevron; it wears the select's hover, focus and disabled state.
    box: [
      "box-border h-control-form rounded-control bg-surface-raised px-control-timeline-inline",
      "font-sans text-body text-text hover:bg-surface-raised-hover has-[:focus-visible]:shadow-focus",
      "has-[:disabled]:opacity-disabled",
      "[[data-surface-raised]_&]:bg-surface [[data-surface-raised]_&]:hover:bg-surface-hover",
    ],
    // The select itself, over the field, unseen.
    select: "cursor-pointer opacity-0 disabled:cursor-not-allowed",
    chevron: "text-muted-text",
    number: [
      "box-border h-control-form rounded-control border-0 bg-surface-raised px-control-timeline-inline",
      "text-end font-sans text-body text-text tabular-nums outline-hidden focus-visible:shadow-focus",
      "[[data-surface-raised]_&]:bg-surface",
    ],
    unit: "text-body text-muted-text",
  },
});

/**
 * The field of a select, as `as`: the value, which takes the room and is cut where it does not fit, the chevron at
 * its end, and the select itself (`select`) over all of it. It is as wide as what holds it, and no wider than its
 * value needs where that is sized by what it holds.
 */
function SelectFrame({
  as,
  value,
  chevron,
  select,
}: {
  as: ReactElement<{ className?: unknown }>;
  value: ReactNode;
  chevron: ReactNode;
  select: ReactElement<{ className?: unknown }>;
}) {
  return (
    <Box as={as} className="relative flex w-full min-w-0 items-center gap-entry-caption">
      <span className="min-w-0 flex-1 truncate">{value}</span>
      <span className="flex flex-none items-center">{chevron}</span>
      <Box as={select} className="absolute inset-0 size-full" />
    </Box>
  );
}

/** The number's field, as `as`: wide enough for a few figures. */
function NumberFrame({ as }: { as: ReactElement<{ className?: unknown }> }) {
  return <Box as={as} className="w-[5rem]" />;
}

export type SelectOption = { value: string; label: string };
/** Options under a name of their own (the time zones of a region). */
export type SelectGroup = { label: string; options: readonly SelectOption[] };

export type SelectProps = {
  id?: string | undefined;
  /** The name it posts under in a form, when it is in one. */
  name?: string | undefined;
  /** Its name, where no label of the page names it (`labelledBy`). */
  label?: string | undefined;
  labelledBy?: string | undefined;
  describedBy?: string | undefined;
  options: readonly (SelectOption | SelectGroup)[];
  /** Its value, where its owner keeps it, with `onChange`; else it keeps its own from `defaultValue`. */
  value?: string | undefined;
  defaultValue?: string | undefined;
  onChange?: ((value: string) => void) | undefined;
  isDisabled?: boolean | undefined;
};

const option = (o: SelectOption) => (
  <option key={o.value} value={o.value}>
    {o.label}
  </option>
);

const flat = (options: readonly (SelectOption | SelectGroup)[]): readonly SelectOption[] =>
  options.flatMap((o) => ("options" in o ? o.options : [o]));

/**
 * A choice from a list, long or short: the platform's own picker, closed as a field as wide as its value, a chevron
 * at its end.
 */
export function Select({
  id,
  name,
  label,
  labelledBy,
  describedBy,
  options,
  value,
  defaultValue,
  onChange,
  isDisabled,
}: SelectProps) {
  const s = field();
  const all = flat(options);
  // The value it shows is the chosen option's words, so it keeps the choice itself where its owner does not.
  const [own, setOwn] = useState(defaultValue ?? all[0]?.value ?? "");
  const chosen = value ?? own;
  return (
    <SelectFrame
      as={<span className={s.box()} />}
      value={all.find((o) => o.value === chosen)?.label ?? ""}
      chevron={
        <span className={s.chevron()}>
          <Icon name="expand" place="caption" />
        </span>
      }
      select={
        <select
          id={id}
          name={name}
          className={s.select()}
          aria-label={label}
          aria-labelledby={labelledBy}
          aria-describedby={describedBy}
          value={chosen}
          disabled={isDisabled}
          onChange={(e) => {
            setOwn(e.currentTarget.value);
            onChange?.(e.currentTarget.value);
          }}
        >
          {options.map((o) =>
            "options" in o ? (
              <optgroup key={o.label} label={o.label}>
                {o.options.map(option)}
              </optgroup>
            ) : (
              option(o)
            ),
          )}
        </select>
      }
    />
  );
}

export type NumberFieldProps = {
  id?: string | undefined;
  name?: string | undefined;
  /** Its name, where no label of the page names it (`labelledBy`). */
  label?: string | undefined;
  labelledBy?: string | undefined;
  describedBy?: string | undefined;
  value?: string | undefined;
  defaultValue?: string | undefined;
  onChange?: ((value: string) => void) | undefined;
  min?: number | undefined;
  max?: number | undefined;
  /** What it counts, after the field ("days"). */
  unit?: string | undefined;
};

/** A number (days, a threshold): a short field, its figures at its end, and its unit after it. */
export function NumberField({
  id,
  name,
  label,
  labelledBy,
  describedBy,
  value,
  defaultValue,
  onChange,
  min,
  max,
  unit,
}: NumberFieldProps) {
  const s = field();
  return (
    <Row as="span" inline align="center" gap="entry-caption">
      <NumberFrame
        as={
          <input
            id={id}
            name={name}
            className={s.number()}
            type="text"
            inputMode="numeric"
            aria-label={label}
            aria-labelledby={labelledBy}
            aria-describedby={describedBy}
            value={value}
            defaultValue={defaultValue}
            min={min}
            max={max}
            {...(onChange === undefined
              ? {}
              : {
                  onChange: (e: { currentTarget: { value: string } }) => {
                    onChange(e.currentTarget.value);
                  },
                })}
          />
        }
      />
      {unit !== undefined && <span className={s.unit()}>{unit}</span>}
    </Row>
  );
}
