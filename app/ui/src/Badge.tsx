// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef } from "react";
import { useLocale } from "react-aria-components";
import { tv, type VariantProps } from "tailwind-variants";
import { VisuallyHidden } from "./VisuallyHidden";

const badge = tv({
  base: "inline-flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full px-1.5 align-middle text-xs leading-none font-extrabold tabular-nums",
  variants: {
    tone: {
      accent: "bg-accent-fill text-accent-fill-fg",
      danger: "bg-danger text-danger-fg",
      neutral: "bg-surface-sunken text-fg",
      /** The raised hands of a voice chat call. */
      hand: "bg-hand text-hand-fg",
    },
  },
  defaultVariants: { tone: "accent" },
});

export type BadgeProps = Omit<ComponentPropsWithRef<"span">, "children"> &
  VariantProps<typeof badge> & {
    /** The number to show. Hiding the badge at zero is the caller's decision. */
    count: number;
    /**
     * What the number means, read instead of the drawn digits ("12 unread notifications"). It is
     * required because a bare number has no meaning out of context; the caller formats it with the full count.
     */
    label: string;
    /** The largest number drawn as is; above it the badge shows `max` followed by a plus sign. */
    max?: number;
  };

/**
 * A small count, such as unread notifications. The digits are drawn for sighted users and hidden
 * from assistive technology, which reads `label` instead, so a capped "99+" never hides the real count.
 */
export function Badge({ count, label, max = 99, tone, className, ...props }: BadgeProps) {
  const { locale } = useLocale();
  const format = new Intl.NumberFormat(locale);
  const shown = count > max ? `${format.format(max)}+` : format.format(count);
  return (
    <span {...props} className={badge({ tone, className })}>
      <span aria-hidden="true">{shown}</span>
      <VisuallyHidden>{label}</VisuallyHidden>
    </span>
  );
}
