// SPDX-License-Identifier: AGPL-3.0-only
import type { Ref } from "react";
import { useProgressBar } from "react-aria";
import { tv, type VariantProps } from "tailwind-variants";

const spinner = tv({
  slots: {
    base: "inline-flex shrink-0 align-middle text-accent",
    // A decorative track and a quarter arc in the current color (accent, or the caller's text color
    // through className). With reduced motion the arc stands still and the accessible name still
    // says that something is loading (web-client/README.md D6).
    ring: "rounded-full border-border border-t-current motion-safe:animate-spin",
  },
  variants: {
    size: {
      sm: { ring: "size-4 border-2" },
      md: { ring: "size-6 border-3" },
      lg: { ring: "size-10 border-4" },
    },
  },
  defaultVariants: { size: "md" },
});

export type SpinnerProps = VariantProps<typeof spinner> & {
  /** What is in progress ("Loading posts"). It is the progress bar's accessible name. */
  label: string;
  className?: string;
  ref?: Ref<HTMLSpanElement>;
};

/**
 * An indeterminate progress indicator: an indeterminate `progressbar` named by `label`. It spins only
 * under `motion-safe`. Inside a region that is loading, the region also sets `aria-busy="true"`.
 * It is a `span` (React Aria's `useProgressBar` rather than the `div` of its `ProgressBar`), so that it
 * is phrasing content and may sit in a paragraph, a label, or a button.
 */
export function Spinner({ label, size, className, ref }: SpinnerProps) {
  const { base, ring } = spinner({ size });
  const { progressBarProps } = useProgressBar({ "aria-label": label, isIndeterminate: true });
  return (
    <span {...progressBarProps} ref={ref} className={base({ className })}>
      <span aria-hidden="true" className={ring()} />
    </span>
  );
}
