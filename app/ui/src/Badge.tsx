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
    /** The largest number drawn as is; above it the badge shows `max` followed by a plus sign. */
    max?: number;
    /** The count is a lower bound (a list not loaded to its end): the number is followed by a plus sign. */
    atLeast?: boolean;
    /** The most there may be (a list with a limit): the count is drawn over it ("3 / 50"), and `max` does not apply. */
    limit?: number;
  } & (
    | {
        /**
         * What the number means, read instead of the drawn digits ("12 unread notifications"). It is
         * required because a bare number has no meaning out of context; the caller formats it with the full count.
         */
        label: string;
        decorative?: false;
      }
    | {
        /**
         * The badge sits in a control that is named elsewhere (`aria-label`, with the count in it): the digits are
         * drawn as generated content, so that they are neither text of the control nor read, and the control's
         * visible text stays its label (WCAG 2.5.3).
         */
        decorative: true;
        label?: never;
      }
  );

/**
 * A small count, such as unread notifications. The digits are drawn for sighted users and hidden
 * from assistive technology, which reads `label` instead, so a capped "99+" never hides the real count.
 */
export function Badge({
  count,
  label,
  decorative = false,
  max = 99,
  atLeast = false,
  limit,
  tone,
  className,
  ...props
}: BadgeProps) {
  const { locale } = useLocale();
  const format = new Intl.NumberFormat(locale);
  let shown = count > max ? `${format.format(max)}+` : `${format.format(count)}${atLeast ? "+" : ""}`;
  if (limit !== undefined) shown = `${format.format(count)} / ${format.format(limit)}`;
  if (decorative) {
    return (
      <span
        {...props}
        aria-hidden="true"
        data-count={shown}
        className={badge({ tone, className: ["after:content-[attr(data-count)]", className] })}
      />
    );
  }
  return (
    <span {...props} className={badge({ tone, className })}>
      <span aria-hidden="true">{shown}</span>
      <VisuallyHidden>{label}</VisuallyHidden>
    </span>
  );
}
