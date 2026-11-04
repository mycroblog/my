// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef } from "react";
import { tv } from "tailwind-variants";
import type { BadgeColor } from "./tokens";

const roleBadge = tv({
  base: "inline-block max-w-full rounded-full px-1.5 py-0.5 align-middle text-xs font-semibold wrap-anywhere",
  variants: {
    color: {
      neutral: "bg-badge-neutral text-badge-neutral-fg",
      red: "bg-badge-red text-badge-red-fg",
      orange: "bg-badge-orange text-badge-orange-fg",
      yellow: "bg-badge-yellow text-badge-yellow-fg",
      green: "bg-badge-green text-badge-green-fg",
      teal: "bg-badge-teal text-badge-teal-fg",
      blue: "bg-badge-blue text-badge-blue-fg",
      purple: "bg-badge-purple text-badge-purple-fg",
      pink: "bg-badge-pink text-badge-pink-fg",
    } satisfies Record<BadgeColor, string>,
  },
});

export type RoleBadgeProps = Omit<ComponentPropsWithRef<"span">, "children" | "color"> & {
  /**
   * The name on the badge. It is the staff's own text, the same in every locale and in no recorded
   * language, so it is drawn as user content (`dir="auto"`, `lang=""`) and never translated.
   */
  label: string;
  /** The badge's color, one of the token pairs. It only decorates: the label carries the meaning. */
  color: BadgeColor;
};

/** The badge of a role, such as a supporter's plan (ADR-0070): a label on one of the badge colors. */
export function RoleBadge({ label, color, className, ...props }: RoleBadgeProps) {
  return (
    <span dir="auto" lang="" {...props} className={roleBadge({ color, className })}>
      {label}
    </span>
  );
}

export type RoleBadgeListProps = Omit<ComponentPropsWithRef<"ul">, "children"> & {
  /** The accessible name of the list, such as "Badges". */
  label: string;
  /** The badges in their order, each with a stable ID. */
  badges: readonly { id: string; label: string; color: BadgeColor }[];
};

/**
 * The badges that an account shows on its profile, as a list with an accessible name. It renders nothing
 * when there are none.
 */
export function RoleBadgeList({ label, badges, className, ...props }: RoleBadgeListProps) {
  if (badges.length === 0) return null;
  return (
    <ul {...props} aria-label={label} className={roleBadgeList({ className })}>
      {badges.map((b) => (
        <li key={b.id} className="max-w-full">
          <RoleBadge label={b.label} color={b.color} />
        </li>
      ))}
    </ul>
  );
}

const roleBadgeList = tv({ base: "flex flex-wrap items-center gap-1.5" });
