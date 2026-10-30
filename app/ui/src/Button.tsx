// SPDX-License-Identifier: AGPL-3.0-only
import {
  Button as RACButton,
  type ButtonProps as RACButtonProps,
  composeRenderProps,
  Link as RACLink,
  type LinkProps as RACLinkProps,
} from "react-aria-components";
import { tv, type VariantProps } from "tailwind-variants";

const button = tv({
  base: [
    "inline-flex cursor-default items-center justify-center gap-2 rounded-full text-center font-extrabold select-none",
    "data-disabled:opacity-50 data-focus-visible:focus-ring data-pending:cursor-wait",
    "motion-safe:transition-[color,background-color,box-shadow,translate,scale] motion-safe:duration-fast motion-safe:ease-standard",
    // The button sizes a child icon, which sets no size of its own (web-client/16-look-states-and-wording.md G11).
    "pointer-coarse:min-h-11 [&>svg]:shrink-0",
  ],
  variants: {
    // Pressed, every button answers at once (web-client/21-experience-principles.md P2). A raised one (the soft surfaces,
    // README "Shape, elevation and states") sinks: its raised shadow turns into the inner shadow of a well and its fill
    // changes, which also answers with reduced motion. A flat one (outline, ghost, subtle) dents, and with reduced
    // motion its pressed fill answers. Forced-colors mode drops the shadows; the system draws the button there.
    variant: {
      /** The one main action of a view: filled with the accent, raised. */
      primary: [
        "bg-accent-fill text-accent-fill-fg shadow-soft-raised data-hovered:bg-accent-fill-hover motion-safe:data-hovered:-translate-y-px",
        "data-pressed:bg-accent-fill-hover data-pressed:shadow-soft-inset",
      ],
      /** Other actions: a raised face with a `border-strong` boundary (3:1). */
      secondary: [
        "border border-border-strong bg-surface-raised text-accent shadow-soft-raised data-hovered:text-accent-hover",
        "data-pressed:bg-surface-sunken data-pressed:text-accent-hover data-pressed:shadow-soft-inset",
      ],
      /** A rule and the accent color on a clear ground: the follow button. */
      outline:
        "border border-accent text-accent data-hovered:bg-soft data-pressed:bg-soft motion-safe:data-pressed:scale-95",
      /** Low-emphasis actions in toolbars and lists. */
      ghost: "text-fg data-hovered:bg-surface-sunken data-pressed:bg-surface-sunken motion-safe:data-pressed:scale-95",
      /** A low-emphasis action beside content, in the muted text color: the actions of a post's toolbar that are not toggles, the button that lists who reacted. */
      subtle:
        "text-fg-muted data-hovered:bg-surface-sunken data-pressed:bg-surface-sunken motion-safe:data-pressed:scale-95",
      /** Destructive actions: filled with the danger color, raised. */
      danger: "bg-danger text-danger-fg shadow-soft-raised data-pressed:shadow-soft-inset",
    },
    size: {
      sm: "min-h-8 px-3 text-sm [&>svg]:size-4",
      md: "min-h-10 px-4 text-base [&>svg]:size-5",
      /** The calls to action of a hero. */
      lg: "min-h-12 px-6 text-base [&>svg]:size-5",
      /** Among `ToggleButton`s in a row of actions (a post's): as tall as they are, with its icon as large. */
      toolbar: "min-h-8 px-2 text-sm [&>svg]:size-5",
      /** Beside a button of the same height, with only an icon in it: the height of `md` with narrow sides. */
      compact: "min-h-10 px-2 text-base [&>svg]:size-5",
      /** Only an icon, named by `aria-label`: as wide as it is tall, and a tap target on a coarse pointer (web-client/README.md D4). */
      icon: "min-h-8 w-8 px-0 text-sm pointer-coarse:w-11 [&>svg]:size-4",
    },
  },
  defaultVariants: { variant: "secondary", size: "md" },
});

export type ButtonProps = RACButtonProps &
  VariantProps<typeof button> & {
    /**
     * The keys that do what pressing the button does (`aria-keyshortcuts`), e.g. `Control+Enter Meta+Enter` on the
     * submit button of a form that Ctrl / Cmd + Enter sends. The button only announces them; the keys are handled where
     * they are pressed.
     */
    "aria-keyshortcuts"?: string;
  };

/**
 * A button (React Aria Button): pressed with a pointer, Enter or Space. Its accessible name is its
 * children; a button of only an icon is an `IconButton`, which names it and shows the name as a tooltip (G12). While
 * `isPending` it ignores presses but keeps its focus.
 *
 * A submit button takes the focus when a pointer press ends rather than when it starts
 * (web-client/16-look-states-and-wording.md G23). A field that loses the focus commits its validation (React Aria's
 * native validation updates its error on blur), so moving the focus at the start of the press would let a field
 * above drop its error and move the button from under the pointer before it is released, which loses the press and
 * the submission.
 */
export function Button({ variant, size, className, "aria-keyshortcuts": keyShortcuts, ...props }: ButtonProps) {
  const submitProps: Partial<RACButtonProps> =
    props.type === "submit" && props.preventFocusOnPress === undefined
      ? {
          preventFocusOnPress: true,
          onPress: (event) => {
            // The focus the browser would have moved on pointer down, now that the press is over. Enter and Space on the
            // button already have it there, and Enter in a field (the form's implicit submission, a `virtual` click)
            // keeps it in the field. The form submits after this, in the click's default action.
            const pointer =
              event.pointerType === "mouse" || event.pointerType === "touch" || event.pointerType === "pen";
            if (pointer && event.target instanceof HTMLElement) event.target.focus();
            props.onPress?.(event);
          },
        }
      : {};
  return (
    <RACButton
      {...props}
      {...submitProps}
      // React Aria passes no aria-keyshortcuts to the DOM, so the button element is rendered here to carry it.
      {...(keyShortcuts !== undefined && {
        render: (domProps) => (
          <button {...domProps} type={domProps.type ?? "button"} aria-keyshortcuts={keyShortcuts} />
        ),
      })}
      className={composeRenderProps(className, (className) => button({ variant, size, className }))}
    />
  );
}

export type ButtonLinkProps = RACLinkProps & VariantProps<typeof button>;

/**
 * A link (`<a href>`) that looks like a `Button`, for a call to action that leaves the app or opens another page (the
 * registration page). Activated as a link: Enter, not Space.
 */
export function ButtonLink({ variant, size, className, ...props }: ButtonLinkProps) {
  return (
    <RACLink
      {...props}
      className={composeRenderProps(className, (className) => button({ variant, size, className }))}
    />
  );
}
