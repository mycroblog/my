// SPDX-License-Identifier: AGPL-3.0-only
// The parts every form field shares: its label, description and error message, and the box of a
// text control. React Aria Components wires the ids (aria-labelledby / aria-describedby) through
// context, so these only style the RAC parts; the field components compose them.
import type { ReactNode } from "react";
import { FieldError as RACFieldError, Label as RACLabel, Text, type ValidationResult } from "react-aria-components";
import { tv } from "tailwind-variants";
import { WarningIcon } from "./Icons";

const field = tv({
  slots: {
    label: "text-sm font-bold text-fg",
    description: "text-sm text-fg-muted",
    error: "flex items-start gap-1 text-sm font-semibold text-danger [&>svg]:mt-1 [&>svg]:size-4 [&>svg]:shrink-0",
  },
});

/** The box of a text control (an input, a text area); the field recipes put it in their control slot. */
export const fieldControl = tv({
  base: [
    "w-full min-w-0 rounded-md border border-border-strong bg-surface-sunken px-3 py-2 text-base text-fg",
    "placeholder:text-fg-muted data-focused:focus-ring data-invalid:border-danger data-invalid:ring-1 data-invalid:ring-danger data-disabled:cursor-not-allowed data-disabled:opacity-50",
    "pointer-coarse:min-h-11 motion-safe:transition-colors",
  ],
});

/** What a field shows when it is invalid: fixed content, or content computed from the validation. */
export type FieldErrorMessage = ReactNode | ((validation: ValidationResult) => ReactNode);

/** The label, description and error message props of every field. Text arrives only through props (web-client/README.md A2). */
export interface FieldTextProps {
  /** The visible label. Without one, pass `aria-label` or `aria-labelledby`. */
  label?: ReactNode;
  /** Help text under the control, linked to it with aria-describedby. */
  description?: ReactNode;
  /**
   * Shown, and linked with aria-describedby, while the field is invalid (`isInvalid`, or a failed
   * `validate` / native constraint). Without it, the browser's validation message is shown.
   */
  errorMessage?: FieldErrorMessage;
}

interface PartProps<T> {
  children: T;
  /** Merged into the part's classes with tailwind-merge. */
  className?: string;
}

export function FieldLabel({ children, className }: PartProps<ReactNode>) {
  return <RACLabel className={field().label({ className })}>{children}</RACLabel>;
}

export function FieldDescription({ children, className }: PartProps<ReactNode>) {
  return (
    <Text slot="description" className={field().description({ className })}>
      {children}
    </Text>
  );
}

export function FieldError({ children, className }: PartProps<FieldErrorMessage>) {
  // The warning icon keeps an error from resting on colour alone.
  return (
    <RACFieldError className={field().error({ className })}>
      {(validation) => {
        const message =
          typeof children === "function" ? children(validation) : (children ?? validation.validationErrors.join(" "));
        if (message === undefined || message === null || message === false || message === "") return null;
        return (
          <>
            <WarningIcon filled />
            <span>{message}</span>
          </>
        );
      }}
    </RACFieldError>
  );
}
