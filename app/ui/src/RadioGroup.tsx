// SPDX-License-Identifier: AGPL-3.0-only
import type { ReactNode } from "react";
import {
  composeRenderProps,
  RadioButton,
  RadioField,
  type RadioFieldProps,
  RadioGroup as RACRadioGroup,
  type RadioGroupProps as RACRadioGroupProps,
} from "react-aria-components";
import { tv } from "tailwind-variants";
import { FieldDescription, FieldError, FieldLabel, type FieldTextProps } from "./Field";

const radioGroup = tv({
  slots: {
    root: "flex flex-col gap-2",
    items: "flex gap-x-6 gap-y-1",
  },
  variants: {
    orientation: {
      vertical: { items: "flex-col" },
      horizontal: { items: "flex-row flex-wrap" },
    },
  },
  defaultVariants: { orientation: "vertical" },
});

export interface RadioGroupProps extends Omit<RACRadioGroupProps, "children">, FieldTextProps {
  /** The `Radio`s. */
  children?: ReactNode;
}

/**
 * A radio group (React Aria RadioGroup): Tab moves into and out of the group, the arrow keys move and
 * select. Controlled with `value` / `onChange` or uncontrolled with `defaultValue`.
 */
export function RadioGroup({
  label,
  description,
  errorMessage,
  children,
  className,
  orientation = "vertical",
  ...props
}: RadioGroupProps) {
  const styles = radioGroup({ orientation });
  return (
    <RACRadioGroup
      {...props}
      orientation={orientation}
      className={composeRenderProps(className, (className) => styles.root({ className }))}
    >
      {label !== undefined && <FieldLabel>{label}</FieldLabel>}
      <div className={styles.items()}>{children}</div>
      {description !== undefined && <FieldDescription>{description}</FieldDescription>}
      <FieldError>{errorMessage}</FieldError>
    </RACRadioGroup>
  );
}

const radio = tv({
  slots: {
    root: "group flex flex-col gap-1",
    button: [
      "flex min-h-6 cursor-default items-center gap-3 text-base text-fg",
      "data-disabled:cursor-not-allowed data-disabled:opacity-50 pointer-coarse:min-h-11",
    ],
    circle: [
      "flex size-5 shrink-0 items-center justify-center rounded-full border-2 border-border-strong bg-surface-sunken",
      "group-data-invalid:border-danger motion-safe:transition-colors",
    ],
    dot: "hidden size-2.5 rounded-full bg-accent group-data-invalid:bg-danger",
    // Aligns the description with the label text: the circle (1.25rem) plus the gap (0.75rem).
    text: "ms-8",
  },
  variants: {
    isSelected: { true: { circle: "border-accent group-data-invalid:border-danger", dot: "block" } },
    isFocusVisible: { true: { circle: "focus-ring" } },
  },
});

export interface RadioProps extends Omit<RadioFieldProps, "children"> {
  /** The label. */
  children?: ReactNode;
  /** Help text for this option, linked to it with aria-describedby. */
  description?: ReactNode;
}

/** One option of a `RadioGroup` (React Aria RadioField); `value` identifies it. */
export function Radio({ children, description, className, ...props }: RadioProps) {
  const styles = radio();
  return (
    <RadioField {...props} className={composeRenderProps(className, (className) => styles.root({ className }))}>
      <RadioButton className={styles.button()}>
        {({ isSelected, isFocusVisible }) => {
          const state = radio({ isSelected, isFocusVisible });
          return (
            <>
              <span className={state.circle()}>
                <span className={state.dot()} />
              </span>
              {children}
            </>
          );
        }}
      </RadioButton>
      {description !== undefined && <FieldDescription className={styles.text()}>{description}</FieldDescription>}
    </RadioField>
  );
}
