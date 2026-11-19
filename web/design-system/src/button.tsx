// SPDX-License-Identifier: AGPL-3.0-only

// The button (ADR-0025 §4), on React Aria Components' Button: its kinds told apart by surfaces, not outlines: the
// primary is the accent fill, the secondary a neutral raised surface, the quiet one has none until touched. React Aria
// tells hover, press and keyboard focus apart (data-hovered, data-pressed, data-focus-visible); the classes read only
// the theme of theme.ts, generated from @my/design-tokens. The icon button (icon-button.tsx) and the chip (chip.tsx)
// draw on the same base. The text button is an action inside a line of text, underlined in the text's own color, and
// the text link a link drawn as it is.

import { motion } from "@my/design-tokens";
import type { ComponentProps, ReactNode } from "react";
import { Button as AriaButton, Link as AriaLink, type ButtonProps as AriaButtonProps } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Box, Pin, Row, type Gap, type RowProps } from "./layout";
import { usePreviewState } from "./preview";

export type ButtonKind = "primary" | "secondary" | "quiet";
/**
 * Compact in a timeline, roomier in a form, or a timeline's words in a finger's target at every pointer (`target`: an
 * action of a row or of a screen's head that a mouse presses too, Manage the event, a list's Revoke).
 */
export type ButtonSize = "timeline" | "form" | "target";

export type ButtonProps = Omit<AriaButtonProps, "className" | "style" | "render" | "isPending" | "children"> & {
  /** The primary action of a view (one at most), a secondary one, or a quiet one such as Cancel. */
  kind?: ButtonKind;
  /** Compact in a timeline, roomier in a form. */
  size?: ButtonSize;
  /** An action in flight: the button keeps its width and its label as its name, shows a spinner, and ignores presses. */
  busy?: boolean;
  /** The keys that run the button's action (what `useAction` returns), for assistive technology. */
  "aria-keyshortcuts"?: string | undefined;
  /**
   * On a touch screen, its press target reaches the touch height above and below it, as an icon button's does: a
   * button of a row that is a finger's target tall (a list's Remove), drawn a timeline control's height.
   */
  touch?: boolean | undefined;
  children: ReactNode;
};

/**
 * The attribute a surface that marks itself (a hovered or focused post) sets, so that an icon button inside it steps
 * its hover a tone down and still shows; `markedSurface` gives it.
 */
const marked = "data-surface-marked";

/** The props that mark a surface while `on`, for the icon buttons inside it: spread them on the surface. */
export const markedSurface = (on: boolean): Record<typeof marked, "" | undefined> => ({
  [marked]: on ? "" : undefined,
});

/**
 * The attribute a raised surface that holds controls sets (a section of a settings screen), so that a neutral control
 * on it (a secondary button, a field) takes the page's surface and still shows, as a field of the composer's
 * card does; spread it on the surface.
 */
export const raisedSurface = { "data-surface-raised": "" } as const;

// What a neutral control wears on a raised surface: the page's surface, and its hover and press.
const onRaised = [
  "[[data-surface-raised]_&]:bg-surface [[data-surface-raised]_&]:data-hovered:bg-surface-hover",
  "[[data-surface-raised]_&]:data-pressed:bg-surface-pressed",
].join(" ");

const button = tv({
  slots: {
    base: [
      "box-border cursor-pointer rounded-control border-0 font-sans leading-none",
      "data-pressed:translate-y-press data-focus-visible:shadow-focus data-focus-visible:outline-hidden",
      "data-disabled:cursor-not-allowed data-disabled:opacity-disabled data-pending:cursor-progress",
    ],
    label: "",
    spinner: [
      "box-border size-spinner rounded-full border-spinner border-solid border-spinner-track border-t-current",
      "animate-spin",
      motion.spin.reduced === "still" ? "motion-reduce:animate-none" : "",
    ],
  },
  variants: {
    kind: {
      primary: {
        base: "text-body font-label bg-accent text-on-accent data-hovered:bg-accent-hover data-pressed:bg-accent-pressed",
      },
      secondary: {
        base: [
          "text-body font-label bg-surface-raised text-text data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed",
          onRaised,
        ],
      },
      quiet: {
        base: "text-body font-label bg-transparent text-text data-hovered:bg-surface-hover data-pressed:bg-surface-pressed",
      },
      // Muted until pressed or on; inside a marked surface, which wears surface-hover, its hover steps to
      // surface-pressed and its press to surface-raised-hover.
      icon: {
        base: [
          "text-caption bg-transparent text-muted-text aria-pressed:text-text data-pressed:text-text",
          "data-hovered:bg-surface-hover data-pressed:bg-surface-pressed",
          "[[data-surface-marked]_&]:data-hovered:bg-surface-pressed",
          "[[data-surface-marked]_&]:data-pressed:bg-surface-raised-hover",
        ],
      },
      // A toggle on the raised surface; on, it is knocked out: the text's color as its fill, with no hue.
      chip: {
        base: [
          "text-caption bg-surface-raised text-text data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed",
          "aria-pressed:bg-text aria-pressed:text-surface",
          "aria-pressed:data-hovered:bg-text aria-pressed:data-pressed:bg-text",
        ],
      },
    },
    // A button in words is a control's height; a label longer than the room its container leaves it wraps, and once
    // its lines are taller than the control the button grows with them, a little padding around them. A button of a
    // glyph, of a count or of the frame keeps its height and its one line.
    size: {
      timeline: { base: "min-h-control-timeline px-control-timeline-inline py-control-timeline-block" },
      form: { base: "min-h-control-form px-control-form-inline py-control-form-block" },
      target: { base: "min-h-icon-button-touch px-control-timeline-inline py-control-timeline-block" },
      icon: { base: "h-control-timeline px-icon-button-inline py-0 whitespace-nowrap" },
      // A glyph alone that is a finger's target at every pointer: the action of a row of a table.
      square: { base: "size-icon-button-touch px-0 py-0 whitespace-nowrap" },
      chip: { base: "h-chip px-chip-inline py-0 whitespace-nowrap" },
      // The app frame's primary action (shell.tsx): the width of the rail or the sidebar, or floating over the tab bar.
      side: { base: "h-shell-item px-0 py-0 whitespace-nowrap" },
      float: { base: "size-shell-bar px-0 py-0 whitespace-nowrap" },
      // A notice floating over a feed (the new entries' button): a timeline button's type and padding at the full
      // touch height, since it is pressed over moving content.
      notice: { base: "h-icon-button-touch px-control-timeline-inline py-0 whitespace-nowrap" },
    },
    busy: { true: { label: "text-transparent" } },
  },
});

type Kind = ButtonKind | "icon" | "chip";
type Size = ButtonSize | "icon" | "square" | "chip" | "side" | "float" | "notice";

/**
 * How a button places what it holds: its label in its middle, under the spinner while busy. It keeps its size, which
 * is its label's on one line, up to the width of what holds it. An icon button's press target reaches the touch height on a touch screen; the frame's
 * action in the rail is as wide as the rail.
 */
const arrangement = (kind: Kind, size: Size, touch = false) =>
  ({
    inline: true,
    align: "center",
    justify: "center",
    anchor: true,
    rigid: true,
    touch: kind === "icon" || touch,
    full: size === "side",
  }) satisfies RowProps;

/**
 * The label: the button's words, or, where a button holds more than words (an icon and its count, a chip's glyph and
 * count, the frame's action), its parts in a row, a gap apart.
 */
function Label({
  kind,
  size,
  className,
  children,
}: {
  kind: Kind;
  size: Size;
  className: string | undefined;
  children: ReactNode;
}) {
  const gap: Gap | undefined =
    size === "side" ? "shell-inset" : kind === "icon" ? "icon-button-gap" : kind === "chip" ? "chip-gap" : undefined;
  if (gap === undefined && size !== "float") return <span className={className}>{children}</span>;
  return (
    <Row as="span" inline align="center" gap={gap} className={className}>
      {children}
    </Row>
  );
}

/**
 * The pointerdown a React Aria button hears: none while it is disabled. A browser gives a disabled button the
 * pointer's events and no click; React Aria, which ends a press at its click, would hold that press, and take the first
 * press once the button is enabled for its end.
 */
export const heardPointerDown = (props: ComponentProps<"button">) =>
  props.disabled === true ? undefined : props.onPointerDown;

/** The base of the button and the icon button: a kind and a size, and a toggle's state. */
export function ButtonBase({
  kind,
  size,
  busy = false,
  pressed,
  "aria-keyshortcuts": keyshortcuts,
  "aria-description": description,
  touch,
  children,
  ...rest
}: Omit<ButtonProps, "kind" | "size"> & {
  kind: Kind;
  size: Size;
  pressed?: boolean | undefined;
  /** What assistive technology reads after the name (an icon button's exact count). */
  "aria-description"?: string | undefined;
}) {
  const preview = usePreviewState();
  const s = button({ kind, size, busy });
  return (
    <Row
      {...arrangement(kind, size, touch)}
      as={
        <AriaButton
          {...rest}
          isPending={busy}
          className={s.base()}
          render={(props, { isHovered, isPressed, isFocusVisible }) => (
            <button
              {...props}
              onPointerDown={heardPointerDown(props)}
              aria-pressed={pressed}
              aria-keyshortcuts={keyshortcuts}
              aria-description={description}
              aria-busy={busy || undefined}
              data-hovered={isHovered || preview === "hover" || undefined}
              data-pressed={isPressed || preview === "pressed" || undefined}
              data-focus-visible={isFocusVisible || preview === "focus-visible" || undefined}
            />
          )}
        />
      }
    >
      <Label kind={kind} size={size} className={s.label()}>
        {children}
      </Label>
      {busy && <Pin as="span" center className={s.spinner()} aria-hidden="true" />}
    </Row>
  );
}

export function Button({ kind = "secondary", size = "form", ...rest }: ButtonProps) {
  return <ButtonBase kind={kind} size={size} {...rest} />;
}

export type LinkButtonProps = {
  href: string;
  kind?: ButtonKind;
  size?: ButtonSize;
  children: ReactNode;
};

/**
 * A link that leads to another page and looks like a button of its kind (signing in with a login provider, whose
 * start is a page of its own): it keeps a link's semantics, so it opens with a page load and in a new tab.
 */
export function LinkButton({ href, kind = "secondary", size = "form", children }: LinkButtonProps) {
  const s = button({ kind, size });
  return (
    <Row {...arrangement(kind, size)} as={<AriaLink href={href} className={`${s.base()} no-underline`} />}>
      <Label kind={kind} size={size} className={s.label()}>
        {children}
      </Label>
    </Row>
  );
}

// It takes the font and the color of the line it sits in, and keeps no surface, so the line reads as one. Its press
// target reaches the target minimum above and below the text (its padding), and it reaches as far past its line, which
// gives the room back, so the line keeps its height.
const textButton = tv({
  base: [
    "cursor-pointer rounded-control border-0 bg-transparent px-0 py-target-line text-current [font:inherit]",
    "underline underline-offset-[0.15em] outline-hidden data-focus-visible:shadow-focus",
  ],
});

export type TextButtonProps = Omit<AriaButtonProps, "className" | "style" | "render" | "isPending" | "children"> & {
  children: ReactNode;
};

/** An action inside a line of text (a translation's "Show original"), underlined in the line's own color. */
export function TextButton({ children, ...rest }: TextButtonProps) {
  const preview = usePreviewState();
  return (
    <Box
      bleedBlock="target-line"
      as={
        <AriaButton
          {...rest}
          className={textButton()}
          render={(props, { isFocusVisible }) => (
            <button
              {...props}
              onPointerDown={heardPointerDown(props)}
              data-focus-visible={isFocusVisible || preview === "focus-visible" || undefined}
            />
          )}
        />
      }
    >
      {children}
    </Box>
  );
}

/**
 * A link inside a line of text that leads to another screen (a notice's Sign in), drawn as the text button is, so that
 * an action and a link in a line read alike.
 */
export function TextLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Box bleedBlock="target-line" as={<AriaLink href={href} className={textButton()} />}>
      {children}
    </Box>
  );
}
