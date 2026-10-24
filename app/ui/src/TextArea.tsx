// SPDX-License-Identifier: AGPL-3.0-only
import { type FocusEvent, type ReactNode, type RefObject, useEffect, useId, useRef, useState } from "react";
import {
  composeRenderProps,
  TextArea as RACTextArea,
  type TextAreaProps as RACTextAreaProps,
  TextField as RACTextField,
  type TextFieldProps as RACTextFieldProps,
} from "react-aria-components";
import { tv } from "tailwind-variants";
import { FieldDescription, FieldError, FieldLabel, type FieldTextProps, fieldControl } from "./Field";
import { type TextHighlight, TextAreaMirror } from "./TextAreaMirror";

const textArea = tv({
  slots: {
    root: "flex flex-col gap-1",
    input: [fieldControl(), "min-h-24 resize-y"],
    // The box of the text area and its end action (endAction): the action sits in the top end corner. The room the
    // text leaves for it is geometry, not a spacing step: the button of size `icon` (32 px, 44 px wide on a coarse
    // pointer) and its inset.
    control: "relative min-w-0 [--end-action-room:--spacing(10)] pointer-coarse:[--end-action-room:--spacing(12)]",
    action: "absolute inset-e-1 top-1",
    count: "text-sm text-fg-muted",
    status: "sr-only",
  },
  variants: {
    over: { true: { count: "font-medium text-danger" } },
    // The text keeps clear of the end action, so that no line runs under its button (a button of size `icon`).
    withAction: { true: { input: "pe-(--end-action-room)" } },
    // A text area with layers in its box (the highlight layer, or TextAreaAutocomplete's mirror, which is also it): the box
    // paints the field's background, so the layer behind the transparent textarea shows. The textarea is a block, so
    // the box is exactly its size, and its line height is rounded to whole pixels, so the scrolled layer lines up
    // with it in Chromium and WebKit too (#1152).
    layered: {
      true: {
        control: "rounded-md bg-surface-sunken",
        input: "relative block bg-transparent leading-[round(calc(var(--text-base--line-height)*1em),1px)]",
      },
    },
  },
  compoundVariants: [
    // The layered textarea is `dir="auto"`, so its own inline end is the other side from the box's when its text runs
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

/** Remaining counts that are announced at once when typing reaches them (ADR-0027). */
const thresholds = [50, 20, 0] as const;

/**
 * How long typing must pause before the count is announced. The GOV.UK character count waits for a
 * pause, because a live region updated on every keystroke interrupts the user's own typing echo.
 */
export const countAnnounceDelay = 1000;

/** The number of thresholds `remaining` has reached; it grows as the text gets closer to the limit. */
function thresholdLevel(remaining: number) {
  return thresholds.filter((threshold) => remaining <= threshold).length;
}

/** Characters as the server counts them: Unicode code points (data-model/README.md). */
function countCharacters(text: string) {
  return Array.from(text).length;
}

type CharacterCountProps =
  | { characterLimit?: undefined; formatRemaining?: undefined }
  | {
      /** The most characters the text may have. Longer text is not cut off; it makes the field invalid. */
      characterLimit: number;
      /** The count message, e.g. "12 characters left" / "3 characters too many"; `remaining` is negative when over. */
      formatRemaining: (remaining: number) => string;
    };

export type TextAreaProps = Omit<RACTextFieldProps, "children"> &
  FieldTextProps &
  CharacterCountProps & {
    placeholder?: RACTextAreaProps["placeholder"];
    rows?: RACTextAreaProps["rows"];
    /**
     * A control in the top end corner of the text area: an icon button of size `icon` that adds to the text (the
     * emoji picker's, ADR-0091 §4). The text keeps clear of it on the end side.
     */
    endAction?: ReactNode;
    /**
     * Ranges of the text to highlight while typing (ADR-0092 §4): their backgrounds are painted on a layer behind the
     * transparent textarea, which draws the text itself; forced-colors mode underlines them instead. The layer is
     * `aria-hidden`, and the textarea is `dir="auto"` with a whole-pixel line height, so the layer lines up with it.
     */
    highlights?: readonly TextHighlight[];
  };

/** The props that TextAreaAutocomplete gives the textarea or input of its field (kit-internal). */
type LayeredInputProps = {
  dir: "auto";
  onSelect: () => void;
  onFocus: () => void;
  onBlur: (event: FocusEvent<HTMLTextAreaElement | HTMLInputElement>) => void;
  onCompositionStart: () => void;
  onCompositionEnd: () => void;
};

/**
 * What TextAreaAutocomplete and TextFieldAutocomplete add to the field: props of its textarea or input and the layers of
 * its box (kit-internal).
 */
export type FieldLayers = {
  /** The ref of the control; the completion holds a textarea or an input in one ref. */
  inputRef: RefObject<HTMLTextAreaElement | HTMLInputElement | null>;
  inputProps: LayeredInputProps;
  /** Rendered in the control's box before it, so it sits behind the control (the mirror). */
  behind: ReactNode;
  /** Rendered in the control's box after it (the anchor of the suggestions). */
  after: ReactNode;
};

/**
 * A multi-line text field (React Aria TextField with a TextArea), controlled with `value` / `onChange`
 * or uncontrolled with `defaultValue`.
 *
 * With `characterLimit`, a count follows the control. It follows the GOV.UK character count: the visible
 * count is linked to the text area with aria-describedby (hidden from the reading order so it is not
 * read twice), and a separate `role="status"` region announces it when typing pauses for
 * `countAnnounceDelay` ms, or at once when the remaining count reaches 50, 20 or 0.
 *
 * With `endAction`, that control sits in the top end corner of the text area (inset-inline-end), and the text keeps
 * clear of it.
 */
export function TextArea(props: TextAreaProps) {
  return <TextAreaField {...props} />;
}

/** TextArea with the layers that TextAreaAutocomplete adds (kit-internal; not exported from the package). */
export function TextAreaField({
  label,
  description,
  errorMessage,
  placeholder,
  rows,
  characterLimit,
  formatRemaining,
  className,
  value,
  defaultValue,
  onChange,
  isInvalid,
  "aria-describedby": ariaDescribedBy,
  endAction,
  highlights,
  layers,
  ...props
}: TextAreaProps & { layers?: FieldLayers | undefined }) {
  const [uncontrolledValue, setUncontrolledValue] = useState(defaultValue ?? "");
  const text = value ?? uncontrolledValue;
  // Highlights without TextAreaAutocomplete's layers (whose mirror draws them) bring a mirror of their own.
  const highlightRef = useRef<HTMLTextAreaElement>(null);
  const ownMirror = layers === undefined && highlights !== undefined;
  const layered = layers !== undefined || ownMirror;
  // The remaining count to announce after the pause, and the one the status region says now.
  const [pending, setPending] = useState<number | null>(null);
  const [announced, setAnnounced] = useState<number | null>(null);
  const countId = useId();

  const remaining = characterLimit === undefined ? undefined : characterLimit - countCharacters(text);
  const over = remaining !== undefined && remaining < 0;

  useEffect(() => {
    if (pending === null) return;
    const timer = setTimeout(() => {
      setAnnounced(pending);
      setPending(null);
    }, countAnnounceDelay);
    return () => {
      clearTimeout(timer);
    };
  }, [pending]);

  function handleChange(next: string) {
    if (value === undefined) setUncontrolledValue(next);
    if (characterLimit !== undefined && remaining !== undefined) {
      const nextRemaining = characterLimit - countCharacters(next);
      if (thresholdLevel(nextRemaining) > thresholdLevel(remaining)) {
        setPending(null);
        setAnnounced(nextRemaining);
      } else {
        setPending(nextRemaining);
      }
    }
    onChange?.(next);
  }

  const styles = textArea({ over, withAction: endAction !== undefined, layered });
  const input = (
    <RACTextArea
      {...layers?.inputProps}
      {...(ownMirror ? { dir: "auto" } : {})}
      // The layers of a text area hold its textarea.
      ref={(layers?.inputRef ?? highlightRef) as RefObject<HTMLTextAreaElement | null>}
      className={styles.input()}
      {...(placeholder === undefined ? {} : { placeholder })}
      {...(rows === undefined ? {} : { rows })}
    />
  );
  const describedBy = [ariaDescribedBy, remaining === undefined ? undefined : countId].filter((id) => id !== undefined).join(" ");

  return (
    <RACTextField
      {...props}
      value={text}
      onChange={handleChange}
      // Only a true isInvalid is passed: any defined value overrides React Aria's own validation (`validate`,
      // `isRequired`), which would then never mark the field invalid or stop its form.
      {...(isInvalid === true || over ? { isInvalid: true } : {})}
      {...(describedBy === "" ? {} : { "aria-describedby": describedBy })}
      className={composeRenderProps(className, (className) => styles.root({ className }))}
    >
      {label !== undefined && <FieldLabel>{label}</FieldLabel>}
      {endAction === undefined && !layered ? (
        input
      ) : (
        <div className={styles.control()}>
          {layers?.behind}
          {ownMirror && <TextAreaMirror textareaRef={highlightRef} text={text} caret={null} highlights={highlights} />}
          {input}
          {layers?.after}
          {endAction !== undefined && <div className={styles.action()}>{endAction}</div>}
        </div>
      )}
      {description !== undefined && <FieldDescription>{description}</FieldDescription>}
      {remaining !== undefined && formatRemaining !== undefined && (
        <>
          <span id={countId} aria-hidden className={styles.count()}>
            {formatRemaining(remaining)}
          </span>
          <span role="status" className={styles.status()}>
            {announced === null ? "" : formatRemaining(announced)}
          </span>
        </>
      )}
      <FieldError>{errorMessage}</FieldError>
    </RACTextField>
  );
}
