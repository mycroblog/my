// SPDX-License-Identifier: AGPL-3.0-only

// The button (ADR-0025 §4), on React Aria Components' Button: its kinds told apart by surfaces, not outlines: the
// primary is the accent fill, the secondary a neutral raised surface, the quiet one has none until touched. React Aria
// tells hover, press and keyboard focus apart (data-hovered, data-pressed, data-focus-visible); the classes read only
// the theme of theme.ts, generated from @my/design-tokens.

import { motion } from "@my/design-tokens";
import type { ReactNode } from "react";
import { Button as AriaButton, type ButtonProps as AriaButtonProps } from "react-aria-components";
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
  children: ReactNode;
};

const button = tv({
  slots: {
    base: [
      "relative box-border inline-flex items-center justify-center whitespace-nowrap",
      "cursor-pointer rounded-control border-0 py-0 font-sans text-body leading-none font-label",
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
      primary: { base: "bg-accent text-on-accent data-hovered:bg-accent-hover data-pressed:bg-accent-pressed" },
      secondary: {
        base: "bg-surface-raised text-text data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed",
      },
      quiet: { base: "bg-transparent text-text data-hovered:bg-surface-hover data-pressed:bg-surface-pressed" },
    },
    size: {
      timeline: { base: "h-control-timeline px-control-timeline-inline" },
      form: { base: "h-control-form px-control-form-inline" },
    },
    busy: { true: { label: "text-transparent" } },
  },
});

export function Button({ kind = "secondary", size = "form", busy = false, children, ...rest }: ButtonProps) {
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
