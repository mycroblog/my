// SPDX-License-Identifier: AGPL-3.0-only
// A post that the viewer's keyword filters fold (ADR-0157 §2, web-client/13-status-detail-and-actions.md): in its place,
// an article named only by the line that says which filters it matched, with the button that shows it. Nothing of the
// post is rendered, so neither the eye nor a screen reader meets its author, text, or media until it is shown; inside
// a Feed it is one of the feed's articles, and Page Up / Page Down step over it as over any post.
import { type ReactNode, useId } from "react";
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { FeedArticle, type FeedArticleProps } from "./Feed";
import { FilterIcon } from "./Icons";

const filteredPost = tv({
  slots: {
    // The card of a post (PostCard), with the room of one line.
    base: "@container mb-3 rounded-xl border border-border bg-surface-raised px-4 py-3 shadow-card",
    row: "flex flex-wrap items-center gap-x-3 gap-y-2",
    icon: "flex size-5 shrink-0 items-center justify-center text-fg-muted [&>svg]:size-5",
    text: "min-w-0 flex-1 text-sm wrap-anywhere text-fg-muted",
  },
});

export type FilteredPostProps = Omit<
  FeedArticleProps,
  "children" | "aria-labelledby" | "aria-describedby" | "aria-label"
> & {
  /** What the post matched ("Filtered: Spoilers"), the article's name. The filters' names are the user's text. */
  label: ReactNode;
  /** The button's words ("Show"). */
  showLabel: string;
  /** Shows the post: the caller puts the post's own card in this one's place. */
  onShow: () => void;
};

/** The line and the button that stand in for a folded post. */
export function FilteredPost({ label, showLabel, onShow, className, ...props }: FilteredPostProps) {
  const slots = filteredPost();
  const labelId = useId();
  return (
    <FeedArticle {...props} aria-labelledby={labelId} className={slots.base({ className })}>
      <div className={slots.row()}>
        <span aria-hidden="true" className={slots.icon()}>
          <FilterIcon />
        </span>
        <p id={labelId} className={slots.text()}>
          {label}
        </p>
        <Button size="sm" onPress={onShow}>
          {showLabel}
        </Button>
      </div>
    </FeedArticle>
  );
}
