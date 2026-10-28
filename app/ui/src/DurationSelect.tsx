// SPDX-License-Identifier: AGPL-3.0-only
// How long an action lasts (a suspension, a silence, a signup block): a `Select` of fixed durations, with
// "indefinitely" when the action may have no end. The value is a number of seconds, or null for no end, so
// the caller adds it to the moment of the request to get the end (`expires_at`, data-model/README.md).
import { Select, SelectItem, type SelectProps } from "./Select";

/** The option id of "no end"; the other options' ids are their seconds. */
const indefinite = "indefinite";

type Option = { id: string; seconds: number | null; label: string };

export type DurationSelectProps = Omit<
  SelectProps<Option>,
  "items" | "children" | "value" | "defaultValue" | "onChange"
> & {
  /** The durations to choose from, in seconds, in the order to show them. */
  durations: readonly number[];
  /** A duration in words, in the viewer's locale ("7 days"). */
  formatDuration: (seconds: number) => string;
  /** The label of "no end" ("Indefinitely"). Without it, every choice has an end. */
  indefiniteLabel?: string | undefined;
  /** The selected duration: seconds, `null` for no end, or `undefined` for none yet (the placeholder). */
  value?: number | null | undefined;
  defaultValue?: number | null | undefined;
  onChange?: (seconds: number | null) => void;
};

function toKey(seconds: number | null | undefined): string | null | undefined {
  if (seconds === undefined) return undefined;
  return seconds === null ? indefinite : String(seconds);
}

/**
 * A duration select (React Aria Select, the APG select-only combobox). "No end" comes last, after the
 * durations, so that the default reading order goes from the lightest choice to the heaviest.
 */
export function DurationSelect({
  durations,
  formatDuration,
  indefiniteLabel,
  value,
  defaultValue,
  onChange,
  ...props
}: DurationSelectProps) {
  const options: Option[] = durations.map((seconds) => ({
    id: String(seconds),
    seconds,
    label: formatDuration(seconds),
  }));
  if (indefiniteLabel !== undefined) options.push({ id: indefinite, seconds: null, label: indefiniteLabel });
  const selected = toKey(value);
  const initial = toKey(defaultValue);
  return (
    <Select<Option>
      {...props}
      items={options}
      {...(selected === undefined ? {} : { value: selected })}
      {...(initial === undefined ? {} : { defaultValue: initial })}
      onChange={(key) => {
        const option = options.find((item) => item.id === key);
        if (option !== undefined) onChange?.(option.seconds);
      }}
    >
      {(option) => <SelectItem id={option.id}>{option.label}</SelectItem>}
    </Select>
  );
}
