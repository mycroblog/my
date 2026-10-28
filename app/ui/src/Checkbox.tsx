// SPDX-License-Identifier: AGPL-3.0-only
import type { ReactNode } from "react";
import { CheckboxButton, CheckboxField, type CheckboxFieldProps, composeRenderProps } from "react-aria-components";
import { tv } from "tailwind-variants";
import { FieldDescription, FieldError, type FieldTextProps } from "./Field";

const checkbox = tv({
  slots: {
    root: "group flex flex-col gap-1",
    button: [
      "flex min-h-6 cursor-default items-center gap-3 text-base text-fg",
      "data-disabled:cursor-not-allowed data-disabled:opacity-50 pointer-coarse:min-h-11",
    ],
    box: [
      "flex size-5 shrink-0 items-center justify-center rounded-sm border-2 border-border-strong bg-surface-sunken text-accent-fg",
      "group-data-invalid:border-danger motion-safe:transition-colors",
    ],
    icon: "size-4 fill-none stroke-current stroke-3",
    // Aligns the description and error with the label text: the box (1.25rem) plus the gap (0.75rem).
    text: "ms-8",
  },
  variants: {
    isSelected: {
      true: {
        box: "border-accent bg-accent group-data-invalid:border-danger group-data-invalid:bg-danger group-data-invalid:text-danger-fg",
      },
    },
    isFocusVisible: { true: { box: "focus-ring" } },
  },
});

export interface CheckboxProps extends Omit<CheckboxFieldProps, "children">, Omit<FieldTextProps, "label"> {
  /** The label. */
  children?: ReactNode;
}

/**
 * A checkbox (React Aria CheckboxField): toggled with a pointer or Space. Controlled with `isSelected` /
 * `onChange` or uncontrolled with `defaultSelected`; `isIndeterminate` shows the mixed state.
 */
export function Checkbox({ children, description, errorMessage, className, ...props }: CheckboxProps) {
  const styles = checkbox();
  return (
    <CheckboxField {...props} className={composeRenderProps(className, (className) => styles.root({ className }))}>
      <CheckboxButton className={styles.button()}>
        {({ isSelected, isIndeterminate, isFocusVisible }) => (
          <>
            <span className={checkbox({ isSelected: isSelected || isIndeterminate, isFocusVisible }).box()}>
              {(isSelected || isIndeterminate) && (
                <svg viewBox="0 0 24 24" aria-hidden className={styles.icon()}>
                  {isIndeterminate ? <path d="M5 12h14" /> : <path d="m5 12 5 5 9-10" />}
                </svg>
              )}
            </span>
            {children}
          </>
        )}
      </CheckboxButton>
      {description !== undefined && <FieldDescription className={styles.text()}>{description}</FieldDescription>}
      <FieldError className={styles.text()}>{errorMessage}</FieldError>
    </CheckboxField>
  );
}
