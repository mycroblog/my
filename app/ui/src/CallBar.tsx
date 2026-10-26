// SPDX-License-Identifier: AGPL-3.0-only
// The small bar of a voice chat call that the user is in (web-client/09-app-shell.md §9), which stays in view on
// every page while the call lasts: the room's name (a link back to its page) with the call's state, and its controls
// (the microphone, leaving). It is a region named by `label`, so that assistive technology lists it with the
// landmarks. Place it at the end of the page in the shell's `ShellDock`, which keeps it at the bottom edge of the
// screen, clear of the tab bar and the floating new-post button.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";

const callBar = tv({
  slots: {
    base: "flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-border bg-surface-raised px-4 py-2 shadow-md",
    // The room's name takes the room left, and is cut short rather than push the controls off the bar.
    heading: "flex min-w-0 flex-1 basis-40 flex-col",
    title: "min-w-0 truncate font-bold",
    status: "text-sm text-fg-muted",
    actions: "flex flex-wrap items-center gap-2",
  },
});

export type CallBarProps = {
  /** The bar's accessible name ("Voice chat call"). */
  label: string;
  /** The room, as a link to its page. */
  title: ReactNode;
  /** The call's state, in a few words ("Connected", "Reconnecting…"). */
  status?: ReactNode;
  /** The controls: the microphone switch, the leave button. */
  children: ReactNode;
};

export function CallBar({ label, title, status, children }: CallBarProps) {
  const styles = callBar();
  return (
    <section aria-label={label} data-call-bar="" className={styles.base()}>
      <div className={styles.heading()}>
        <div className={styles.title()}>{title}</div>
        {status !== undefined && <div className={styles.status()}>{status}</div>}
      </div>
      <div className={styles.actions()}>{children}</div>
    </section>
  );
}
