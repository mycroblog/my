// SPDX-License-Identifier: AGPL-3.0-only

// The layout of a thread (FR-106) in a feed: the focused post, a step larger, a group's space before and after it, with
// its own lines under its body (when it was posted and its counts); and each reply under it, indented by how deep it
// replies, up to three steps where the column is wide enough and none where it is narrow (a phone's), where the
// reply's own context line says to whom.

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Icon, type IconName } from "./icons";

const focus = tv({
  base: [
    "my-screen-focus-gap grid grid-cols-[minmax(0,1fr)] gap-screen-focus-gap",
    // Its name and its text, which the content renderer sizes itself, a step larger, at the body's weight and leading.
    "[&_[data-entry]]:text-[length:var(--text-title)]",
    "[&_[data-entry]_div.text-body]:text-[length:var(--text-title)]",
    "[&_[data-entry]_div.text-body]:leading-[var(--text-body--line-height)]",
  ],
});

/** The focused post of a thread, a step larger, apart from the posts around it by a group's space. */
export function ThreadFocus({ children }: { children: ReactNode }) {
  return <div className={focus()}>{children}</div>;
}

const reply = tv({
  slots: { root: "@container", indent: "grid grid-cols-[minmax(0,1fr)] gap-feed" },
  variants: {
    depth: {
      0: {},
      1: { indent: "@reply:ms-screen-reply-step" },
      2: { indent: "@reply:ms-[calc(2*var(--screen-reply-step))]" },
      3: { indent: "@reply:ms-[calc(3*var(--screen-reply-step))]" },
    },
  },
});

/** A reply under the focused post (`depth` 1 replies to it), and what goes with it (a fold of its branch). */
export function ThreadReply({ depth, children }: { depth: number; children: ReactNode }) {
  const s = reply({ depth: Math.max(0, Math.min(3, depth - 1)) as 0 | 1 | 2 | 3 });
  return (
    <div className={s.root()}>
      <div className={s.indent()}>{children}</div>
    </div>
  );
}

const details = tv({
  slots: {
    root: "grid gap-entry-line text-caption text-muted-text [&_p]:m-0",
    glyph: "inline-flex items-center gap-entry-caption align-bottom",
    count: [
      "rounded-control text-body text-text no-underline outline-hidden",
      "data-hovered:underline data-focus-visible:shadow-focus",
    ],
  },
});

/** The focused post's own lines under its body: each a `<p>`, muted and small. */
export function DetailLines({ children }: { children: ReactNode }) {
  return <div className={details().root()}>{children}</div>;
}

/** A glyph and its words inside a detail line (the post's visibility). */
export function GlyphText({ icon, children }: { icon: IconName; children: ReactNode }) {
  return (
    <span className={details().glyph()}>
      <Icon name={icon} place="caption" />
      {children}
    </span>
  );
}

/** A count inside a detail line, in the text's color. */
export function DetailCount({ children }: { children: ReactNode }) {
  return <span className={details().count()}>{children}</span>;
}
