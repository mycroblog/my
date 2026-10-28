// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef, ReactNode } from "react";
import { tv } from "tailwind-variants";
import { ContentWarning } from "./ContentWarning";

const quoteCard = tv({
  slots: {
    base: "flex w-full min-w-0 flex-col gap-2 rounded-md border border-border bg-surface px-3 py-2",
    header: "flex min-w-0 items-center gap-2 text-sm",
    author: "min-w-0 flex-1",
    time: "shrink-0 text-fg-muted",
    body: "flex min-w-0 flex-col gap-2 text-sm",
    notice: "min-w-0 text-sm wrap-anywhere text-fg-muted",
  },
});

export type QuoteCardProps = Omit<ComponentPropsWithRef<"div">, "children"> & {
  /** The accessible name of the box ("Quoted post"). */
  label: string;
  /** The quoted author's avatar, at the start of the header row. */
  avatar?: ReactNode;
  /** The quoted author's name and handle. */
  author?: ReactNode;
  /** When the quoted post was written, usually a link to its page. */
  time?: ReactNode;
  /** The quoted post's body and media; without it, the box shows only the notice. */
  children?: ReactNode;
  /**
   * Why the quoted post is not shown ("The quoted post was deleted."). With children and `revealLabel`, the
   * post is behind it, as behind a content warning: the reader's mute of its author or server.
   */
  notice?: ReactNode;
  /** The name of the button that shows a post behind the notice ("Show the quoted post"). */
  revealLabel?: string;
};

/**
 * The post that a post quotes (FR-114): a box inside the quoting post with the quoted author, time, and body,
 * named as a group. A quoted post that cannot be shown leaves only the notice of why; one whose author or
 * server the reader muted is behind that notice, in the disclosure of ContentWarning (web-client/README.md Y8).
 */
export function QuoteCard({
  label,
  avatar,
  author,
  time,
  children,
  notice,
  revealLabel,
  className,
  ...props
}: QuoteCardProps) {
  const slots = quoteCard();
  const post =
    children === undefined ? null : (
      <>
        <div className={slots.header()}>
          {avatar}
          <div className={slots.author()}>{author}</div>
          {time !== undefined && <div className={slots.time()}>{time}</div>}
        </div>
        <div className={slots.body()}>{children}</div>
      </>
    );
  let inside: ReactNode = post;
  if (post === null) {
    inside = <p className={slots.notice()}>{notice}</p>;
  } else if (notice !== undefined && revealLabel !== undefined) {
    inside = (
      <ContentWarning summary={notice} toggleLabel={revealLabel}>
        {post}
      </ContentWarning>
    );
  }
  return (
    <div role="group" aria-label={label} {...props} className={slots.base({ className })}>
      {inside}
    </div>
  );
}
