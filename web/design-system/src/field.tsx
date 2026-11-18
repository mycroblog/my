// SPDX-License-Identifier: AGPL-3.0-only

// Two fields a setting needs beside the text field and the switch. A choice from a long list (a language, a time
// zone) is the platform's own picker, a select drawn closed as a field with a chevron at its end: on a phone the
// system's list, which a finger scrolls and searches as everywhere else on the device; on a desktop a list the
// keyboard jumps in by typing a name; nothing of ours to load, to translate or to keep accessible, and a list of
// hundreds costs no more than one of three. A number (days, a threshold) is a short field whose figures line up at its
// end, with its unit after it. Both are a control's height, on the neutral surface of a control, and on a raised
// surface (a section of a settings screen) take the page's, as every field does.

import type { ReactElement, ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Icon } from "./icons";
import { Box, Row } from "./layout";

const field = tv({
  slots: {
    select: [
      "box-border h-control-form cursor-pointer appearance-none rounded-control border-0",
      "bg-surface-raised ps-control-timeline-inline font-sans text-body text-text outline-hidden",
      "hover:bg-surface-raised-hover focus-visible:shadow-focus disabled:cursor-not-allowed disabled:opacity-disabled",
      "[[data-surface-raised]_&]:bg-surface [[data-surface-raised]_&]:hover:bg-surface-hover",
    ],
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
 * The select, as `as`, and its chevron over its end, which lets a press through to the select: the select is no wider
 * than what holds it, its value cut where it does not fit, clear of the chevron.
 */
function SelectFrame({ as, chevron }: { as: ReactElement<{ className?: unknown }>; chevron: ReactNode }) {
  return (
    <span className="relative inline-flex max-w-full">
      <Box as={as} className="max-w-full truncate pe-[2.25rem]" />
      <span className="pointer-events-none absolute inset-y-0 end-[0.75rem] flex items-center">{chevron}</span>
    </span>
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

/** A choice from a list, long or short: the platform's own picker, closed as a field with a chevron at its end. */
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
  return (
    <SelectFrame
      chevron={
        <span className={s.chevron()}>
          <Icon name="expand" place="caption" />
        </span>
      }
      as={
        <select
          id={id}
          name={name}
          className={s.select()}
          aria-label={label}
          aria-labelledby={labelledBy}
          aria-describedby={describedBy}
          value={value}
          defaultValue={defaultValue}
          disabled={isDisabled}
          {...(onChange === undefined
            ? {}
            : {
                onChange: (e: { currentTarget: { value: string } }) => {
                  onChange(e.currentTarget.value);
                },
              })}
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
