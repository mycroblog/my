// SPDX-License-Identifier: AGPL-3.0-only
import {
  composeRenderProps,
  ToggleButton as RACToggleButton,
  type ToggleButtonProps as RACToggleButtonProps,
} from "react-aria-components";
import { tv, type VariantProps } from "tailwind-variants";

const toggleButton = tv({
  base: [
    "inline-flex min-h-8 min-w-8 cursor-default items-center justify-center gap-1.5 rounded-md px-2 text-sm font-medium text-fg-muted select-none",
    "data-hovered:bg-surface-sunken data-hovered:text-fg data-pressed:bg-surface-sunken",
    "data-disabled:opacity-50 data-focus-visible:focus-ring motion-safe:transition-colors",
    "pointer-coarse:min-h-11 pointer-coarse:min-w-11 [&>svg]:size-5 [&>svg]:shrink-0",
  ],
  variants: {
    /** The color of the selected state; the shape of the icon should change with it too, not the color alone (WCAG 1.4.1). */
    tone: {
      accent: "data-selected:text-accent data-selected:data-hovered:text-accent-hover",
      success: "data-selected:text-success data-selected:data-hovered:text-success",
      warning: "data-selected:text-warning data-selected:data-hovered:text-warning",
    },
    /** A rule around the button, which turns to the accent color while selected (a reaction chip). */
    outlined: { true: "border border-border data-selected:border-accent" },
  },
  defaultVariants: { tone: "accent" },
});

export type ToggleButtonProps = RACToggleButtonProps & VariantProps<typeof toggleButton>;

/**
 * A two-state button (React Aria ToggleButton; WAI-ARIA APG, button pattern): pressed with a pointer,
 * Enter or Space, it exposes its state with `aria-pressed`. Its accessible name stays the same in both
 * states ("Favourite", not "Unfavourite"), so name it with `aria-label` when its children are an icon
 * and a count, and describe it with the count (`aria-describedby`) if the count matters. Control it with
 * `isSelected` / `onChange`.
 */
export function ToggleButton({ tone, outlined, className, ...props }: ToggleButtonProps) {
  return (
    <RACToggleButton
      {...props}
      className={composeRenderProps(className, (className) => toggleButton({ tone, outlined, className }))}
    />
  );
}
