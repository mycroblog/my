// SPDX-License-Identifier: AGPL-3.0-only
// The small bar of a voice chat call that the user is in (web-client/09-app-shell.md §9), which stays in view on
// every page while the call lasts: the room's name (a link back to its page) with the call's state, its controls
// (the microphone, leaving), and a notice of what failed. It is a region named by `label`, so that assistive
// technology lists it with the landmarks. Place it at the end of the page in the shell's `ShellDock`, which keeps it
// at the bottom edge of the screen, clear of the tab bar and the floating new-post button.
//
// While it shows, its height is the `--call-bar-block-size` of the document (ResizeObserver), from which the app's
// stylesheet keeps focused elements clear of it (scroll-padding): the bar wraps on narrow screens, and the notice
// adds a row.
import { type ReactNode, useEffect, useRef } from "react";
import { tv } from "tailwind-variants";

const callBar = tv({
  slots: {
    base: "flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-border bg-surface-raised px-4 py-2 shadow-md",
    // The room's name takes the room left, and is cut short rather than push the controls off the bar: the link
    // (a flex item, `min-w-0`) shrinks to the room left, so its target never reaches under the controls (WCAG 2.5.8),
    // and the element of its text ends in an ellipsis.
    heading: "flex min-w-0 flex-1 basis-40 flex-col",
    title: "flex min-w-0 font-bold *:min-w-0 [&>*>*]:truncate",
    status: "text-sm text-fg-muted",
    actions: "flex flex-wrap items-center gap-2",
    notice: "basis-full",
  },
});

export type CallBarProps = {
  /** The bar's accessible name ("Voice chat call"). */
  label: string;
  /** The room, as a link to its page, with its text in an element of its own (a `span`), which the bar cuts short. */
  title: ReactNode;
  /** The call's state, in a few words ("Connected", "Reconnecting…"). */
  status?: ReactNode;
  /** The controls: the microphone switch, the leave button. */
  children: ReactNode;
  /** What failed (the microphone, leaving), on a row of its own; nothing for none. */
  notice?: ReactNode;
};

/** The custom property with the bar's height, for the app's scroll-padding. */
const blockSizeProperty = "--call-bar-block-size";

export function CallBar({ label, title, status, children, notice }: CallBarProps) {
  const styles = callBar();
  const ref = useRef<HTMLElement>(null);
  // The bar's height is an external fact that the document's stylesheet needs (react.dev, "Synchronizing with Effects").
  useEffect(() => {
    const element = ref.current;
    if (element === null) return;
    const root = document.documentElement;
    const write = () => {
      root.style.setProperty(blockSizeProperty, `${String(element.getBoundingClientRect().height)}px`);
    };
    write();
    const observer = typeof ResizeObserver === "function" ? new ResizeObserver(write) : null;
    observer?.observe(element);
    return () => {
      observer?.disconnect();
      root.style.removeProperty(blockSizeProperty);
    };
  }, []);
  return (
    <section ref={ref} aria-label={label} data-call-bar="" className={styles.base()}>
      <div className={styles.heading()}>
        <div className={styles.title()}>{title}</div>
        {status !== undefined && <div className={styles.status()}>{status}</div>}
      </div>
      <div className={styles.actions()}>{children}</div>
      {notice !== undefined && notice !== null && notice !== false && <div className={styles.notice()}>{notice}</div>}
    </section>
  );
}
