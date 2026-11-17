// SPDX-License-Identifier: AGPL-3.0-only

// The layout of a thread (FR-106) in a feed: the focused post, a step larger, a group's space before and after it, with
// its own lines under its body (when it was posted, and to whom); and each reply under it, indented by how deep it
// replies, up to three steps where the column is wide enough and none where it is narrow (a phone's), where the
// reply's own context line says to whom.

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Icon, type IconName } from "./icons";
import { Row, Stack } from "./layout";

const focus = tv({
  base: [
    // Its name and its text, which the content renderer sizes itself, a step larger, at the body's weight and leading.
    "[&_[data-entry]]:text-[length:var(--text-title)]",
    "[&_[data-entry]_div.text-body]:text-[length:var(--text-title)]",
    "[&_[data-entry]_div.text-body]:leading-[var(--text-body--line-height)]",
  ],
});

/** The focused post of a thread, a step larger, apart from the posts around it by a group's space. */
export function ThreadFocus({ children }: { children: ReactNode }) {
  return (
    <Stack gap="screen-focus-gap" spaceAbove="screen-focus-gap" spaceBelow="screen-focus-gap" className={focus()}>
      {children}
    </Stack>
  );
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

const details = tv({ base: "text-caption text-muted-text" });

/** The focused post's own lines under its body: each a `<p>`, muted and small. */
export function DetailLines({ children }: { children: ReactNode }) {
  return (
    <Stack gap="entry-line" className={details()}>
      {children}
    </Stack>
  );
}

/** A glyph and its words inside a detail line (the post's visibility). */
export function GlyphText({ icon, children }: { icon: IconName; children: ReactNode }) {
  return (
    <Row as="span" inline align="center" gap="entry-caption" baseline="bottom">
      <Icon name={icon} place="caption" />
      {children}
    </Row>
  );
}
