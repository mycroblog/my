// SPDX-License-Identifier: AGPL-3.0-only
// A non-modal popover dialog anchored to its trigger: React Aria's Popover + Dialog. Focus moves into it
// on open, Esc or a press outside closes it, and focus returns to the trigger (WAI-ARIA APG, dialog pattern).
import type { ReactNode, Ref } from "react";
import {
  Dialog,
  DialogTrigger,
  Heading,
  Popover as AriaPopover,
  type PopoverProps as AriaPopoverProps,
} from "react-aria-components";
import { tv } from "tailwind-variants";

/** The trigger of a Popover: wraps the trigger button and the Popover, and owns the open state. */
export { DialogTrigger as PopoverTrigger };

const popover = tv({
  slots: {
    root: [
      "max-w-[min(24rem,calc(100vw-2rem))] rounded-xl border border-border bg-surface-raised text-fg shadow-lg outline-none",
      "motion-safe:transition motion-safe:duration-fast data-entering:opacity-0 data-exiting:opacity-0",
    ],
    // overscroll-contain keeps the scroll from chaining to the page on iOS WebKit (web-client/09-app-shell.md §9).
    dialog: "flex max-h-[inherit] flex-col gap-2 overflow-y-auto overscroll-contain p-4 outline-none",
    title: "text-base font-extrabold",
  },
});

type Labelled =
  | { /** A visible heading that names the popover. */ title: ReactNode; "aria-label"?: never }
  | { title?: never; "aria-label": string };

export type PopoverProps = Labelled &
  Pick<AriaPopoverProps, "placement" | "offset" | "isOpen" | "defaultOpen" | "onOpenChange" | "triggerRef"> & {
    /** The content. A function receives `close` for actions that close the popover. */
    children: ReactNode | ((renderProps: { close: () => void }) => ReactNode);
    /** Classes merged onto the popover with tailwind-merge. */
    className?: string;
    ref?: Ref<HTMLElement>;
  };

/** A popover with interactive content. Put it inside a `PopoverTrigger` after its trigger button. */
export function Popover({
  title,
  "aria-label": ariaLabel,
  children,
  className,
  ref,
  placement = "bottom",
  ...props
}: PopoverProps) {
  const styles = popover();
  return (
    <AriaPopover {...props} ref={ref} placement={placement} className={styles.root({ class: className })}>
      <Dialog {...(ariaLabel === undefined ? {} : { "aria-label": ariaLabel })} className={styles.dialog()}>
        {({ close }) => (
          <>
            {title !== undefined && (
              <Heading slot="title" className={styles.title()}>
                {title}
              </Heading>
            )}
            {typeof children === "function" ? children({ close }) : children}
          </>
        )}
      </Dialog>
    </AriaPopover>
  );
}
