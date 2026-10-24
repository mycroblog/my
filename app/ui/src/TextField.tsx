// SPDX-License-Identifier: AGPL-3.0-only
import { type ReactNode, type RefObject, useRef, useState } from "react";
import {
  composeRenderProps,
  Input,
  type InputProps,
  TextField as RACTextField,
  type TextFieldProps as RACTextFieldProps,
} from "react-aria-components";
import { tv } from "tailwind-variants";
import { FieldDescription, FieldError, FieldLabel, type FieldTextProps, fieldControl } from "./Field";
import type { FieldLayers } from "./TextArea";
import { type TextHighlight, TextAreaMirror } from "./TextAreaMirror";

const textField = tv({
  slots: {
    root: "flex flex-col gap-1",
    input: fieldControl(),
    // The box of the input and its end action (endAction): the action sits at the end of the input, in the middle
    // of its height (40 px, 44 px on a coarse pointer; the button is 32 px high). The room the text leaves for it is
    // geometry, not a spacing step: the width of the button of size `icon` (32 px, 44 px on a coarse pointer) and its inset.
    control: "relative min-w-0 [--end-action-room:--spacing(10)] pointer-coarse:[--end-action-room:--spacing(12)]",
    action: "absolute inset-e-1 top-1 pointer-coarse:top-1.5",
  },
  variants: {
    grow: { true: { root: "grow" } },
    shrink: { true: { root: "min-w-0" } },
    // The text keeps clear of the end action, so that it does not run under its button (a button of size `icon`).
    withAction: { true: { input: "pe-(--end-action-room)" } },
    // An input with the highlight layer behind it (ADR-0092 §4): the box paints the field's background, so the layer
    // behind the transparent input shows. The input is a block, so the box is exactly its size, and its line height is
    // whole pixels, like the layered text area's (TextArea.tsx, #1152).
    layered: {
      true: {
        control: "rounded-md bg-surface-sunken",
        input: "relative block bg-transparent leading-[round(calc(var(--text-base--line-height)*1em),1px)]",
      },
    },
  },
  compoundVariants: [
    // The layered input is `dir="auto"`, so its own inline end is the other side from the box's when its text runs
    // against the page (English in an Arabic page); the room for the end action then goes on its inline start.
    {
      layered: true,
      withAction: true,
      class: {
        input: [
          "[:dir(ltr)>&:dir(rtl)]:ps-(--end-action-room) [:dir(ltr)>&:dir(rtl)]:pe-3",
          "[:dir(rtl)>&:dir(ltr)]:ps-(--end-action-room) [:dir(rtl)>&:dir(ltr)]:pe-3",
        ],
      },
    },
  ],
});

export interface TextFieldProps extends Omit<RACTextFieldProps, "children">, FieldTextProps {
  placeholder?: InputProps["placeholder"];
  /** Takes the room left in a row (`grow`). */
  grow?: boolean;
  /** Lets the field shrink below its content's width in a row (`min-w-0`), so a long value does not push the row wider. */
  shrink?: boolean;
  /**
   * A control at the end of the input: an icon button of size `icon` that adds to the text (the emoji picker's,
   * ADR-0091 §4). The text keeps clear of it on the end side.
   */
  endAction?: ReactNode;
  /**
   * Ranges of the text to highlight while typing (ADR-0092 §4), as on TextArea: painted on an `aria-hidden` layer behind
   * the transparent input, which is then `dir="auto"` with a whole-pixel line height.
   */
  highlights?: readonly TextHighlight[];
}

/**
 * A single-line text field (React Aria TextField). The label, description and error message are linked
 * to the input; it is controlled with `value` / `onChange` or uncontrolled with `defaultValue`. With `endAction`, that
 * control sits at the end of the input (inset-inline-end), and the text keeps clear of it. With `highlights`, their
 * ranges are painted behind the text.
 */
export function TextField(props: TextFieldProps) {
  return <TextFieldControl {...props} />;
}

/** TextField with the layers that TextFieldAutocomplete adds (kit-internal; not exported from the package). */
export function TextFieldControl({
  label,
  description,
  errorMessage,
  placeholder,
  className,
  grow,
  shrink,
  endAction,
  highlights,
  value,
  defaultValue,
  onChange,
  layers,
  ...props
}: TextFieldProps & { layers?: FieldLayers | undefined }) {
  const [uncontrolledValue, setUncontrolledValue] = useState(defaultValue ?? "");
  const inputRef = useRef<HTMLInputElement>(null);
  // Highlights without TextFieldAutocomplete's layers (whose mirror draws them) bring a mirror of their own.
  const ownMirror = layers === undefined && highlights !== undefined;
  const layered = layers !== undefined || ownMirror;
  const { root, input, control, action } = textField({ grow, shrink, withAction: endAction !== undefined, layered });
  const field = (
    <Input
      {...layers?.inputProps}
      // The layers of a text field hold its input.
      ref={(layers?.inputRef ?? inputRef) as RefObject<HTMLInputElement | null>}
      className={input()}
      {...(ownMirror ? { dir: "auto" } : {})}
      {...(placeholder === undefined ? {} : { placeholder })}
    />
  );
  // The layer needs the text, so a field with highlights keeps an uncontrolled value itself.
  const text = value ?? uncontrolledValue;
  const valueProps: Pick<RACTextFieldProps, "value" | "defaultValue" | "onChange"> = layered
    ? {
        value: text,
        onChange: (next) => {
          if (value === undefined) setUncontrolledValue(next);
          onChange?.(next);
        },
      }
    : {
        ...(value === undefined ? {} : { value }),
        ...(defaultValue === undefined ? {} : { defaultValue }),
        ...(onChange === undefined ? {} : { onChange }),
      };
  return (
    <RACTextField {...props} {...valueProps} className={composeRenderProps(className, (className) => root({ className }))}>
      {label !== undefined && <FieldLabel>{label}</FieldLabel>}
      {endAction === undefined && !layered ? (
        field
      ) : (
        <div className={control()}>
          {layers?.behind}
          {ownMirror && <TextAreaMirror textareaRef={inputRef} text={text} caret={null} highlights={highlights} singleLine />}
          {field}
          {layers?.after}
          {endAction !== undefined && <div className={action()}>{endAction}</div>}
        </div>
      )}
      {description !== undefined && <FieldDescription>{description}</FieldDescription>}
      <FieldError>{errorMessage}</FieldError>
    </RACTextField>
  );
}
