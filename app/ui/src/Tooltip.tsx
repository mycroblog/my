// SPDX-License-Identifier: AGPL-3.0-only
// A tooltip: React Aria's TooltipTrigger + Tooltip (WAI-ARIA APG, tooltip pattern). It opens on hover
// after a delay and at once on keyboard focus, closes on Esc, and describes its trigger
// (aria-describedby). It never holds focus or interactive content; use a Popover for that.
// A tooltip does not replace an accessible name: an icon-only button still needs its own aria-label.
import type { ReactNode, Ref } from "react";
import {
  OverlayArrow,
  Tooltip as AriaTooltip,
  TooltipTrigger,
  type TooltipProps as AriaTooltipProps,
} from "react-aria-components";
import { tv } from "tailwind-variants";

export { TooltipTrigger };
export type { TooltipTriggerComponentProps as TooltipTriggerProps } from "react-aria-components";

const tooltip = tv({
  slots: {
    root: [
      "max-w-64 rounded-md bg-toast px-3 py-1 text-sm font-semibold text-toast-fg shadow-md dark:ring-1 dark:ring-border",
      "motion-safe:transition-opacity motion-safe:duration-fast data-entering:opacity-0 data-exiting:opacity-0",
    ],
    arrow: [
      "block size-2 fill-toast group-data-[placement=bottom]:rotate-180",
      "group-data-[placement=left]:-rotate-90 group-data-[placement=right]:rotate-90",
    ],
  },
});

export type TooltipProps = Pick<AriaTooltipProps, "placement" | "offset"> & {
  children: ReactNode;
  /** Classes merged onto the tooltip with tailwind-merge. */
  className?: string;
  ref?: Ref<HTMLDivElement>;
};

/** The tooltip of a `TooltipTrigger`, placed after the trigger element (a focusable React Aria component). */
export function Tooltip({ children, className, placement = "top", offset = 8, ref }: TooltipProps) {
  const styles = tooltip();
  return (
    <AriaTooltip ref={ref} placement={placement} offset={offset} className={styles.root({ class: className })}>
      <OverlayArrow className="group">
        <svg viewBox="0 0 8 8" aria-hidden="true" className={styles.arrow()}>
          <path d="M0 0 L4 4 L8 0" />
        </svg>
      </OverlayArrow>
      {children}
    </AriaTooltip>
  );
}
