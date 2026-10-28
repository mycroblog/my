// SPDX-License-Identifier: AGPL-3.0-only
// The floating card of a voice chat call that the user is in, on every page but the room's own while the call lasts
// (web-client/09-app-shell.md §9): the stacked avatars of the speakers (the one talking ringed), the room's name (a
// link back to its page) over who is talking or the call's state, its controls (the microphone, leaving), and a
// notice of what failed. It takes the place of `CallBar` and keeps its contract: a region named by `label`, so that
// assistive technology lists it with the landmarks, placed at the end of the page in the shell's `ShellDock`, and
// while it shows its height is the `--call-bar-block-size` of the document, from which the app's stylesheet keeps
// focused elements clear of it.
import { type ReactNode, useRef } from "react";
import { tv } from "tailwind-variants";
import { AvatarStack, type AvatarStackItem } from "./AvatarStack";
import { useCallBarBlockSize } from "./callBarBlockSize";

const miniPlayer = tv({
  slots: {
    base: "flex flex-wrap items-center gap-x-3 gap-y-2 rounded-xl border border-border bg-surface-raised p-2 ps-3 shadow-lg",
    // The room's name and the status take the room left and are cut short rather than push the controls off the
    // card: the link (a flex item, `min-w-0`) shrinks to the room left, so its target never reaches under the
    // controls (WCAG 2.5.8), and the element of its text ends in an ellipsis.
    heading: "flex min-w-0 flex-1 basis-20 flex-col",
    title: "flex min-w-0 text-sm font-extrabold *:min-w-0 [&>*>*]:truncate",
    status: "truncate text-xs text-fg-muted",
    actions: "flex shrink-0 items-center gap-2",
    notice: "basis-full",
  },
});

/** A speaker in the mini-player's stack. */
export type MiniPlayerAvatar = AvatarStackItem;

export type MiniPlayerProps = {
  /** The card's accessible name ("Voice chat call"). */
  label: string;
  /** The speakers to stack, the first on top; the card shows the first three. */
  avatars: readonly MiniPlayerAvatar[];
  /** The room, as a link to its page, with its text in an element of its own (a `span`), which the card cuts short. */
  title: ReactNode;
  /** Who is talking ("Haruka is talking"), or the call's state while it is not connected ("Reconnecting…"). */
  status?: ReactNode;
  /** The controls: `CallDockButton`s with `size="sm"` (the microphone, leaving); none once the call has ended. */
  children?: ReactNode;
  /** What failed (the microphone, leaving), on a row of its own; nothing for none. */
  notice?: ReactNode;
};

/** The floating card of the call, in `ShellDock`. */
export function MiniPlayer({ label, avatars, title, status, children, notice }: MiniPlayerProps) {
  const slots = miniPlayer();
  const ref = useRef<HTMLElement>(null);
  useCallBarBlockSize(ref);
  return (
    <section ref={ref} aria-label={label} data-call-bar="" className={slots.base()}>
      {/* The avatars only decorate: the room's name and the status line say what they show. */}
      <AvatarStack avatars={avatars} />
      <div className={slots.heading()}>
        <div className={slots.title()}>{title}</div>
        {status !== undefined && status !== null && <div className={slots.status()}>{status}</div>}
      </div>
      {children !== undefined && children !== null && <div className={slots.actions()}>{children}</div>}
      {notice !== undefined && notice !== null && notice !== false && <div className={slots.notice()}>{notice}</div>}
    </section>
  );
}
