// SPDX-License-Identifier: AGPL-3.0-only
import type { ReactNode } from "react";
import {
  Button,
  composeRenderProps,
  ListBox,
  ListBoxItem,
  type ListBoxItemProps,
  type ListBoxProps,
  Popover,
  Select as RACSelect,
  type SelectProps as RACSelectProps,
  SelectValue,
} from "react-aria-components";
import { tv } from "tailwind-variants";
import { Tooltip, TooltipTrigger } from "./Tooltip";
import { FieldDescription, FieldError, FieldLabel, type FieldTextProps, fieldControl } from "./Field";

const select = tv({
  slots: {
    root: "group flex flex-col gap-1",
    trigger: "flex cursor-default items-center gap-2 text-start data-focus-visible:focus-ring",
    // The closed select shows only an option's label: its description is for choosing (as React Aria's Select styles do).
    value: "truncate data-placeholder:text-fg-muted **:data-[slot=description]:hidden",
    chevron: "size-4 shrink-0 fill-none stroke-current stroke-2 text-fg-muted",
    // overscroll-contain keeps the scroll from chaining to the page on iOS WebKit (web-client/09-app-shell.md §9).
    popover:
      "min-w-(--trigger-width) overflow-auto overscroll-contain rounded-lg border border-border bg-surface-raised p-1 shadow-md",
    listBox: "outline-hidden",
  },
  variants: {
    variant: {
      field: {
        trigger: [
          fieldControl(),
          "justify-between group-data-invalid:border-danger group-data-invalid:ring-1 group-data-invalid:ring-danger data-pressed:bg-surface-raised",
        ],
      },
      // Looks like a ghost Button of the `toolbar` size, with its icon as large, so that it lines up with the buttons beside it.
      toolbar: {
        root: "min-w-0 flex-row items-center",
        trigger: [
          "min-h-8 min-w-0 rounded-md px-2 text-sm font-medium text-fg select-none",
          "data-disabled:opacity-50 data-hovered:bg-surface-sunken data-pressed:bg-surface-sunken pointer-coarse:min-h-11 motion-safe:transition-colors",
          "[&>svg:first-child]:size-5 [&>svg:first-child]:shrink-0",
        ],
        // Like the other buttons of the row that open a popup, it has no chevron, which also keeps the row short enough for 320 px.
        chevron: "hidden",
      },
    },
  },
  defaultVariants: { variant: "field" },
});

export interface SelectProps<T extends object>
  extends Omit<RACSelectProps<T>, "children" | "selectionMode">, FieldTextProps {
  /** The options, for a dynamic collection rendered by a `children` function. */
  items?: ListBoxProps<T>["items"];
  /** `SelectItem`s, or a function from an item of `items` to its `SelectItem`. */
  children: ReactNode | ((item: T) => ReactNode);
  /**
   * `field` (the default) is a form field under its label. `toolbar` is a small button in a row of tools that
   * shows the chosen option after `icon`; it has no visible label, so name it with `aria-label` (the button's
   * name is then the chosen option followed by that label).
   */
  variant?: "field" | "toolbar";
  /** A decorative icon before the value, for the `toolbar` variant. */
  icon?: ReactNode;
  /**
   * A tooltip on the trigger button, for the `toolbar` variant: it shows on hover and keyboard focus like the
   * tooltips of the icon buttons beside it, closes on Esc, and stays closed while the list is open. It does not
   * replace the accessible name (`aria-label` and the chosen option).
   */
  tooltip?: ReactNode;
}

/**
 * A select (React Aria Select, the APG select-only combobox): Enter, Space or the arrow keys open the
 * list, typing jumps to an option. Controlled with `value` / `onChange` or uncontrolled with `defaultValue`;
 * the value is the selected `SelectItem`'s `id`.
 */
export function Select<T extends object>({
  label,
  description,
  errorMessage,
  items,
  children,
  className,
  variant,
  icon,
  tooltip,
  ...props
}: SelectProps<T>) {
  const styles = select({ variant });
  const trigger = (
    <Button className={styles.trigger()}>
      {icon}
      <SelectValue className={styles.value()} />
      <svg viewBox="0 0 24 24" aria-hidden className={styles.chevron()}>
        <path d="m6 9 6 6 6-6" />
      </svg>
    </Button>
  );
  return (
    <RACSelect {...props} className={composeRenderProps(className, (className) => styles.root({ className }))}>
      {({ isOpen }) => (
        <>
          {label !== undefined && <FieldLabel>{label}</FieldLabel>}
          {tooltip === undefined ? (
            trigger
          ) : (
            <TooltipTrigger isDisabled={isOpen}>
              {trigger}
              <Tooltip>{tooltip}</Tooltip>
            </TooltipTrigger>
          )}
          {description !== undefined && <FieldDescription>{description}</FieldDescription>}
          <FieldError>{errorMessage}</FieldError>
          <Popover className={styles.popover()}>
            <ListBox className={styles.listBox()} {...(items === undefined ? {} : { items })}>
              {children}
            </ListBox>
          </Popover>
        </>
      )}
    </RACSelect>
  );
}

const selectItem = tv({
  slots: {
    root: [
      "flex min-h-8.5 cursor-default items-center gap-2 rounded-md px-2 py-1 text-base text-fg outline-hidden",
      "data-disabled:opacity-50 data-focus-visible:focus-ring data-focused:bg-surface-sunken pointer-coarse:min-h-11",
    ],
    check: "size-4 shrink-0 fill-none stroke-current stroke-3 text-accent",
    // Keeps the label aligned with the selected one, whose check takes this space.
    spacer: "size-4 shrink-0",
    text: "flex min-w-0 flex-col",
    description: "text-sm text-fg-muted",
  },
});

export type SelectItemProps<T extends object> = Omit<ListBoxItemProps<T>, "children"> & {
  /** The option's label. A non-string label needs `textValue` for type-ahead and the closed select. */
  children?: ReactNode;
  /**
   * A line under the label that tells options with similar labels apart. The option's name is all of its visible
   * text, the label then this line (WCAG 2.5.3, label in name), so that speech input can choose it by either; the
   * closed select shows only the label.
   */
  description?: ReactNode;
};

/** Between an option's label and its description in its name: the column layout does not show it. */
const nameSeparator = " ";

/** One option of a `Select`; its `id` is the select's value when it is chosen. */
export function SelectItem<T extends object>({ children, description, className, ...props }: SelectItemProps<T>) {
  const styles = selectItem();
  const textValue = props.textValue ?? (typeof children === "string" ? children : undefined);
  return (
    <ListBoxItem
      {...props}
      {...(textValue === undefined ? {} : { textValue })}
      className={composeRenderProps(className, (className) => styles.root({ className }))}
    >
      {({ isSelected }) => (
        <>
          {isSelected ? (
            <svg viewBox="0 0 24 24" aria-hidden className={styles.check()}>
              <path d="m5 12 5 5 9-10" />
            </svg>
          ) : (
            <span className={styles.spacer()} />
          )}
          {description === undefined ? (
            children
          ) : (
            // Plain text, not React Aria's label and description slots: those would name the option by the label
            // alone, and a visible line outside its name fails WCAG 2.5.3 (label in name).
            <span className={styles.text()}>
              <span>{children}</span>
              {nameSeparator}
              <span data-slot="description" className={styles.description()}>
                {description}
              </span>
            </span>
          )}
        </>
      )}
    </ListBoxItem>
  );
}
