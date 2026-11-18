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
//
// The number is React Aria's number field, a whole number: it takes the figures of the reader's locale (full-width
// ones too) and nothing that is no part of a number; the arrow keys step it by one, Home and End take it to its least
// and its most where it has them, and the wheel does not change it. When the reader leaves it or presses Enter, what
// they entered becomes a number from `min` to `max`, the nearest one where it lay outside, read to assistive
// technology as it changes, or none where the field is empty; the field then tells its owner, once, if the number is
// another than the owner knew. A number given to it from outside what it takes shows, and posts, as the nearest it
// takes, which its owner hears once the reader has been in the field. So its owner hears a number only when the field
// holds one, and a form posts one.
//
// A field in a setting's row takes its name, what describes it and whether it can be changed from the row
// (`FieldContext`), so that no caller ties them by hand.

import { createContext, use, useEffect, useRef, useState, type ReactElement, type ReactNode } from "react";
import { Input as AriaInput, NumberField as AriaNumberField } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Icon } from "./icons";
import { Box, Row } from "./layout";

/** What holds a field gives it (a setting's row, a dialog with what the server refused). */
type FieldHolder = {
  /** The id the field takes where it has none of its own, which the holder's label is for. */
  id?: string | undefined;
  /** The id of the holder's name of the field, where the field has no name of its own. */
  labelledBy?: string | undefined;
  /** The ids of what the holder says of the field, after what the field's own props say. */
  describedBy?: string | undefined;
  /** The field cannot be changed now. */
  disabled?: boolean | undefined;
};

export const FieldContext = createContext<FieldHolder>({});

/** The ids of what describes a field, in order, as `aria-describedby` takes them; undefined for none. */
export const describedBy = (...ids: (string | false | undefined)[]): string | undefined =>
  ids.filter((id) => id !== undefined && id !== false && id !== "").join(" ") || undefined;

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
      "disabled:cursor-not-allowed disabled:opacity-disabled",
      "[[data-surface-raised]_&]:bg-surface",
    ],
    unit: "text-body text-muted-text",
  },
});

/**
 * The field of a select, as `as`: the value, which takes the room and is cut where it does not fit, the chevron at
 * its end, and the select itself (`select`) over all of it. It is as wide as what holds it, and no wider than its
 * value needs where that is sized by what it holds. The value drawn is the select's own, which assistive technology
 * reads from the select alone.
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
      <span className="min-w-0 flex-1 truncate" aria-hidden="true">
        {value}
      </span>
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
  /** Its name, where no label of the page names it (`labelledBy`, or the setting's row it is in). */
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
 * at its end. In a setting's row it is named and described by the row.
 */
export function Select({
  id,
  name,
  label,
  labelledBy,
  describedBy: described,
  options,
  value,
  defaultValue,
  onChange,
  isDisabled,
}: SelectProps) {
  const s = field();
  const holder = use(FieldContext);
  const all = flat(options);
  // The value it shows is the chosen option's words, so it keeps the choice itself where its owner does not.
  const [own, setOwn] = useState(defaultValue ?? all[0]?.value ?? "");
  const chosen = value ?? own;
  const disabled = isDisabled ?? holder.disabled ?? false;
  return (
    <SelectFrame
      // The field drawn is the select's: disabled with it, so that its dimmed value is read as a disabled control's.
      as={<span className={s.box()} aria-disabled={disabled || undefined} />}
      value={all.find((o) => o.value === chosen)?.label ?? ""}
      chevron={
        <span className={s.chevron()}>
          <Icon name="expand" place="caption" />
        </span>
      }
      select={
        <select
          id={id ?? holder.id}
          name={name}
          className={s.select()}
          aria-label={label}
          aria-labelledby={labelledBy ?? (label === undefined ? holder.labelledBy : undefined)}
          aria-describedby={describedBy(described, holder.describedBy)}
          value={chosen}
          disabled={disabled}
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
  /** The name it posts its number under in a form, when it is in one; an empty value while it holds none. */
  name?: string | undefined;
  /** Its name, where no label of the page names it (`labelledBy`, or the setting's row it is in). */
  label?: string | undefined;
  labelledBy?: string | undefined;
  describedBy?: string | undefined;
  /** The number it holds, where its owner keeps it, with `onChange`; `null`, none. Else it keeps its own. */
  value?: number | null | undefined;
  defaultValue?: number | null | undefined;
  /**
   * The whole number from `min` to `max` the field holds once the reader left it or pressed Enter, `null` for an empty
   * field: told once, and only when it is another than its owner knew.
   */
  onChange?: ((value: number | null) => void) | undefined;
  min?: number | undefined;
  max?: number | undefined;
  /** What it counts, after the field ("days"). */
  unit?: string | undefined;
  isDisabled?: boolean | undefined;
};

/** A whole number, written as the reader typed it: no fraction, no grouping. */
const WHOLE: Intl.NumberFormatOptions = { maximumFractionDigits: 0, useGrouping: false };

/**
 * A number (days, a threshold): a short field, its figures at its end, and its unit after it. In a setting's row it
 * is named and described by the row.
 */
export function NumberField({
  id,
  name,
  label,
  labelledBy,
  describedBy: described,
  value,
  defaultValue,
  onChange,
  min,
  max,
  unit,
  isDisabled,
}: NumberFieldProps) {
  const s = field();
  const holder = use(FieldContext);
  // What the field holds, its owner's whenever the owner gives another, and what its owner last gave or heard.
  const [held, setHeld] = useState<number | null>(value === undefined ? (defaultValue ?? null) : value);
  const [known, setKnown] = useState(held);
  const [given, setGiven] = useState(value);
  if (given !== value) {
    setGiven(value);
    if (value !== undefined) {
      setHeld(value);
      setKnown(value);
    }
  }
  // What the reader entered since the owner last heard, until the field tells it.
  const entered = useRef<{ value: number | null } | undefined>(undefined);
  useEffect(() => {
    entered.current = undefined;
  }, [value]);
  const tell = () => {
    const entry = entered.current;
    entered.current = undefined;
    // With nothing entered, the field holds what it was given, or the nearest number it takes, which it shows.
    const now =
      entry !== undefined
        ? entry.value
        : held === null
          ? null
          : Math.min(max ?? Infinity, Math.max(min ?? -Infinity, held));
    if (now === known) return;
    setKnown(now);
    onChange?.(now);
  };
  const ownId = id ?? holder.id;
  const named = labelledBy ?? (label === undefined ? holder.labelledBy : undefined);
  const about = describedBy(described, holder.describedBy);
  return (
    <Row
      inline
      align="center"
      gap="entry-caption"
      as={
        <AriaNumberField
          {...(ownId === undefined ? {} : { id: ownId })}
          {...(name === undefined ? {} : { name })}
          {...(label === undefined ? {} : { "aria-label": label })}
          {...(named === undefined ? {} : { "aria-labelledby": named })}
          {...(about === undefined ? {} : { "aria-describedby": about })}
          value={held ?? Number.NaN}
          onChange={(entry) => {
            const next = Number.isNaN(entry) ? null : entry;
            entered.current = { value: next };
            setHeld(next);
          }}
          onBlur={tell}
          onKeyUp={(e) => {
            if (e.key === "Enter") tell();
          }}
          {...(min === undefined ? {} : { minValue: min })}
          {...(max === undefined ? {} : { maxValue: max })}
          formatOptions={WHOLE}
          isWheelDisabled
          isDisabled={isDisabled ?? holder.disabled ?? false}
        />
      }
    >
      <NumberFrame as={<AriaInput className={s.number()} />} />
      {unit !== undefined && <span className={s.unit()}>{unit}</span>}
    </Row>
  );
}
