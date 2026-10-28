// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef, ReactNode } from "react";
import { tv, type VariantProps } from "tailwind-variants";
import { celebrationClasses } from "./celebration";
import { type PostVisibility, VisibilityIcon } from "./Icons";

const statusBadge = tv({
  base: "inline-flex max-w-full items-center gap-1 rounded-full px-2 py-0.5 align-middle text-xs font-semibold wrap-anywhere [&>svg]:size-4 [&>svg]:shrink-0",
  variants: {
    tone: {
      neutral: "bg-surface-sunken text-fg",
      accent: "bg-accent-fill text-accent-fill-fg",
      success: "bg-success-bg text-success",
      warning: "bg-warning-bg text-warning",
      danger: "bg-danger-bg text-danger",
    },
    // The badge of a state reached by a celebrated moment (web-client/21-experience-principles.md §21.5): its icon bounces
    // a little while the badge is celebrating.
    celebrating: { true: celebrationClasses },
  },
  defaultVariants: { tone: "neutral" },
});

export type StatusBadgeProps = Omit<ComponentPropsWithRef<"span">, "children"> &
  VariantProps<typeof statusBadge> & {
    /**
     * The state's icon from the icon set (web-client/16-look-states-and-wording.md G10). It is required: the shape tells
     * the states apart, not the tone's color alone (WCAG 1.4.1). It is decorative; `label` carries the meaning.
     */
    icon: ReactNode;
    /** The state in words ("Suspended", "Connected"), always shown next to the icon (G12). */
    label: string;
    /**
     * Whether the badge shows the moment a state was just reached, one of the moments of §21.5 (a step of the first run
     * done): `useCelebration`'s `isCelebrating`. Never true for a state shown on load.
     */
    celebrating?: boolean;
  };

/**
 * The state of something in one glance (ADR-0110 P3): an icon, the state's name and a tone. The tone only adds
 * to the icon and the label, which say the state on their own.
 */
export function StatusBadge({ icon, label, tone, celebrating = false, className, ...props }: StatusBadgeProps) {
  return (
    <span
      {...props}
      {...(celebrating && { "data-celebrating": true })}
      className={statusBadge({ tone, celebrating, className })}
    >
      {icon}
      {label}
    </span>
  );
}

const visibilityMark = tv({
  base: "inline-flex max-w-full items-center gap-1 text-xs text-fg-muted wrap-anywhere [&>svg]:size-4 [&>svg]:shrink-0",
});

export type VisibilityMarkProps = Omit<ComponentPropsWithRef<"span">, "children"> & {
  /** Who can see the post (ADR-0017). */
  visibility: PostVisibility;
  /** The level's name ("Followers"), shown next to its icon. */
  label: string;
};

/**
 * A post's visibility: the level's own icon (VisibilityIcon, one shape each) and its name, in the small muted text
 * of the post's secondary information (G6).
 */
export function VisibilityMark({ visibility, label, className, ...props }: VisibilityMarkProps) {
  return (
    <span {...props} className={visibilityMark({ className })}>
      <VisibilityIcon visibility={visibility} />
      {label}
    </span>
  );
}
