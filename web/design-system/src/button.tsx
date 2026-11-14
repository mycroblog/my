// SPDX-License-Identifier: AGPL-3.0-only

// The button (ADR-0025 §4), on React Aria Components' Button: its kinds told apart by surfaces, not outlines: the
// primary is the accent fill, the secondary a neutral raised surface, the quiet one has none until touched. React Aria
// tells hover, press and keyboard focus apart (data-hovered, data-pressed, data-focus-visible); the classes read only
// the theme of theme.ts, generated from @my/design-tokens. The icon button (icon-button.tsx) and the chip (chip.tsx)
// draw on the same base. The text button is an action inside a line of text, underlined in the text's own color.

import { motion } from "@my/design-tokens";
import type { ReactNode } from "react";
import { Button as AriaButton, Link as AriaLink, type ButtonProps as AriaButtonProps } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { usePreviewState } from "./preview";

export type ButtonKind = "primary" | "secondary" | "quiet";
export type ButtonSize = "timeline" | "form";

export type ButtonProps = Omit<AriaButtonProps, "className" | "style" | "render" | "isPending" | "children"> & {
  /** The primary action of a view (one at most), a secondary one, or a quiet one such as Cancel. */
  kind?: ButtonKind;
  /** Compact in a timeline, roomier in a form. */
  size?: ButtonSize;
  /** An action in flight: the button keeps its width and its label as its name, shows a spinner, and ignores presses. */
  busy?: boolean;
  /** The keys that run the button's action (what `useAction` returns), for assistive technology. */
  "aria-keyshortcuts"?: string | undefined;
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

const button = tv({
  slots: {
    base: [
      "relative box-border inline-flex items-center justify-center whitespace-nowrap",
      "cursor-pointer rounded-control border-0 py-0 font-sans leading-none",
      "data-pressed:translate-y-press data-focus-visible:shadow-focus data-focus-visible:outline-hidden",
      "data-disabled:cursor-not-allowed data-disabled:opacity-disabled data-pending:cursor-progress",
    ],
    label: "",
    spinner: [
      "absolute inset-0 m-auto box-border size-spinner rounded-full border-spinner border-solid",
      "border-spinner-track border-t-current animate-spin",
      motion.spin.reduced === "still" ? "motion-reduce:animate-none" : "",
    ],
  },
  variants: {
    kind: {
      primary: {
        base: "text-body font-label bg-accent text-on-accent data-hovered:bg-accent-hover data-pressed:bg-accent-pressed",
      },
      secondary: {
        base: "text-body font-label bg-surface-raised text-text data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed",
      },
      quiet: {
        base: "text-body font-label bg-transparent text-text data-hovered:bg-surface-hover data-pressed:bg-surface-pressed",
      },
      // Muted until pressed or on; inside a marked surface, which wears surface-hover, its hover steps to
      // surface-pressed and its press to surface-raised-hover. The hit area reaches the touch height on a touch screen.
      icon: {
        base: [
          "text-caption bg-transparent text-muted-text aria-pressed:text-text data-pressed:text-text",
          "data-hovered:bg-surface-hover data-pressed:bg-surface-pressed",
          "[[data-surface-marked]_&]:data-hovered:bg-surface-pressed",
          "[[data-surface-marked]_&]:data-pressed:bg-surface-raised-hover",
          "pointer-coarse:before:absolute pointer-coarse:before:inset-x-0 pointer-coarse:before:top-1/2",
          "pointer-coarse:before:h-icon-button-touch pointer-coarse:before:-translate-y-1/2",
        ],
        label: "inline-flex items-center gap-icon-button-gap",
      },
      // A toggle on the raised surface; on, it is knocked out: the text's color as its fill, with no hue.
      chip: {
        base: [
          "text-caption bg-surface-raised text-text data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed",
          "aria-pressed:bg-text aria-pressed:text-surface",
          "aria-pressed:data-hovered:bg-text aria-pressed:data-pressed:bg-text",
        ],
        label: "inline-flex items-center gap-chip-gap",
      },
    },
    size: {
      timeline: { base: "h-control-timeline px-control-timeline-inline" },
      form: { base: "h-control-form px-control-form-inline" },
      icon: { base: "h-control-timeline px-icon-button-inline" },
      chip: { base: "h-chip px-chip-inline" },
      // The app frame's primary action (shell.tsx): the width of the rail or the sidebar, or floating over the tab bar.
      side: { base: "h-shell-item w-full px-0", label: "inline-flex items-center gap-shell-inset" },
      float: { base: "size-shell-bar px-0", label: "inline-flex items-center" },
      // A notice floating over a feed (the new entries' button): a timeline button's type and padding at the full
      // touch height, since it is pressed over moving content.
      notice: { base: "h-icon-button-touch px-control-timeline-inline" },
    },
    busy: { true: { label: "text-transparent" } },
  },
});

/** The base of the button and the icon button: a kind and a size, and a toggle's state. */
export function ButtonBase({
  kind,
  size,
  busy = false,
  pressed,
  "aria-keyshortcuts": keyshortcuts,
  "aria-description": description,
  children,
  ...rest
}: Omit<ButtonProps, "kind" | "size"> & {
  kind: ButtonKind | "icon" | "chip";
  size: ButtonSize | "icon" | "chip" | "side" | "float" | "notice";
  pressed?: boolean | undefined;
  /** What assistive technology reads after the name (an icon button's exact count). */
  "aria-description"?: string | undefined;
}) {
  const preview = usePreviewState();
  const s = button({ kind, size, busy });
  return (
    <AriaButton
      {...rest}
      isPending={busy}
      className={s.base()}
      render={(props, { isHovered, isPressed, isFocusVisible }) => (
        <button
          {...props}
          aria-pressed={pressed}
          aria-keyshortcuts={keyshortcuts}
          aria-description={description}
          aria-busy={busy || undefined}
          data-hovered={isHovered || preview === "hover" || undefined}
          data-pressed={isPressed || preview === "pressed" || undefined}
          data-focus-visible={isFocusVisible || preview === "focus-visible" || undefined}
        />
      )}
    >
      <span className={s.label()}>{children}</span>
      {busy && <span className={s.spinner()} aria-hidden="true" />}
    </AriaButton>
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
    <AriaLink href={href} className={`${s.base()} no-underline`}>
      <span className={s.label()}>{children}</span>
    </AriaLink>
  );
}

// It takes the font and the color of the line it sits in, and keeps no surface, so the line reads as one.
const textButton = tv({
  base: [
    "cursor-pointer rounded-control border-0 bg-transparent p-0 text-current [font:inherit]",
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
    <AriaButton
      {...rest}
      className={textButton()}
      render={(props, { isFocusVisible }) => (
        <button {...props} data-focus-visible={isFocusVisible || preview === "focus-visible" || undefined} />
      )}
    >
      {children}
    </AriaButton>
  );
}
