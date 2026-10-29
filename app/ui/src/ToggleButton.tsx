// SPDX-License-Identifier: AGPL-3.0-only
import {
  composeRenderProps,
  ToggleButton as RACToggleButton,
  type ToggleButtonProps as RACToggleButtonProps,
} from "react-aria-components";
import { tv, type VariantProps } from "tailwind-variants";
import { celebrationClasses, useCelebration } from "./celebration";

const toggleButton = tv({
  base: [
    "inline-flex min-h-8 min-w-8 cursor-default items-center justify-center gap-1.5 rounded-md px-2 text-sm font-medium text-fg-muted select-none",
    "data-hovered:bg-surface-sunken data-hovered:text-fg data-pressed:bg-surface-sunken",
    "data-disabled:opacity-50 data-focus-visible:focus-ring",
    // Pressed, it dents at once (web-client/21-experience-principles.md P2); with reduced motion the sunken fill above answers.
    "motion-safe:transition-[color,background-color,scale] motion-safe:duration-fast motion-safe:ease-standard motion-safe:data-pressed:scale-95",
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
    /** Turning it on is a celebrated moment (web-client/21-experience-principles.md §21.5): the icon bounces a little. */
    celebrate: { true: celebrationClasses },
  },
  defaultVariants: { tone: "accent" },
});

export type ToggleButtonProps = RACToggleButtonProps & VariantProps<typeof toggleButton>;

/**
 * A two-state button (React Aria ToggleButton; WAI-ARIA APG, button pattern): pressed with a pointer,
 * Enter or Space, it exposes its state with `aria-pressed`. Its accessible name stays the same in both
 * states ("Favourite", not "Unfavourite"). When its children are an icon and a count it is an
 * `IconToggleButton`, which names it and shows the name as a tooltip (G12); a count it shows is part of
 * its name (`countedLabel`, WCAG 2.5.3). Control it with `isSelected` / `onChange`.
 *
 * `celebrate` is only for the toggles listed as celebrated moments in §21.5 (a favourite): when the person turns it on,
 * its icon grows in with the small overshoot of `ease-celebrate`, or, with reduced motion, the button takes the `soft`
 * fill for a moment. Turning it off, and a toggle that is already on when it appears, stay quiet.
 */
export function ToggleButton({ tone, outlined, celebrate, className, onChange, ...props }: ToggleButtonProps) {
  const celebration = useCelebration();
  return (
    <RACToggleButton
      {...props}
      {...(celebrate === true && props.isSelected !== false && celebration.celebrationProps)}
      onChange={(isSelected) => {
        if (celebrate === true) {
          if (isSelected) celebration.celebrate();
          else celebration.stop();
        }
        onChange?.(isSelected);
      }}
      className={composeRenderProps(className, (className) => toggleButton({ tone, outlined, celebrate, className }))}
    />
  );
}
