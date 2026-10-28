// SPDX-License-Identifier: AGPL-3.0-only
// Segmented: React Aria's ToggleButtonGroup in single selection (a radiogroup of toggle buttons; the arrow
// keys move between the segments, Tab enters and leaves the group). One segment is always selected.
import {
  ToggleButton as RACToggleButton,
  ToggleButtonGroup as RACToggleButtonGroup,
  type Key,
  type ToggleButtonGroupProps as RACToggleButtonGroupProps,
  type ToggleButtonProps as RACToggleButtonProps,
} from "react-aria-components";
import { tv } from "tailwind-variants";

const segmented = tv({
  slots: {
    group: "inline-flex gap-1 rounded-full border border-border bg-surface-raised p-1",
    item: [
      "inline-flex cursor-default items-center justify-center rounded-full px-3 py-2 text-xs font-bold text-fg-muted select-none",
      "data-hovered:text-fg data-selected:bg-pressed data-selected:text-pressed-fg data-selected:data-hovered:text-pressed-fg",
      "forced-colors:data-selected:bg-[Highlight] forced-colors:data-selected:text-[HighlightText] forced-colors:data-selected:data-hovered:text-[HighlightText]",
      "data-disabled:opacity-50 data-focus-visible:focus-ring motion-safe:transition-colors pointer-coarse:min-h-11",
    ],
  },
});

export type SegmentedProps = Omit<
  RACToggleButtonGroupProps,
  | "className"
  | "style"
  | "selectionMode"
  | "disallowEmptySelection"
  | "selectedKeys"
  | "defaultSelectedKeys"
  | "onSelectionChange"
  | "orientation"
> & {
  /** What the segments switch (“Layout”), for assistive technology. */
  "aria-label": string;
  /** The `id` of the selected segment, when the selection is controlled. */
  value?: Key;
  /** The `id` of the segment that is selected first, when the selection is not controlled. */
  defaultValue?: Key;
  onChange?: (value: Key) => void;
  className?: string;
};

/**
 * A compact pill switcher for two to four views of the same content (the layout of a page, a timeline's density).
 * It is one control with one value: for a set of panels use `Tabs`, for a setting that is on or off a `Switch`.
 */
export function Segmented({ value, defaultValue, onChange, className, ...props }: SegmentedProps) {
  return (
    <RACToggleButtonGroup
      {...props}
      selectionMode="single"
      disallowEmptySelection
      {...(value !== undefined && { selectedKeys: [value] })}
      {...(defaultValue !== undefined && { defaultSelectedKeys: [defaultValue] })}
      onSelectionChange={(keys) => {
        const [key] = keys;
        if (key !== undefined) onChange?.(key);
      }}
      className={segmented().group({ class: className })}
    />
  );
}

export type SegmentedItemProps = Omit<RACToggleButtonProps, "className" | "style" | "id"> & {
  /** The segment's value, as `Segmented` reports it. */
  id: Key;
};

/** One segment of a `Segmented`. */
export function SegmentedItem(props: SegmentedItemProps) {
  return <RACToggleButton {...props} className={segmented().item()} />;
}
