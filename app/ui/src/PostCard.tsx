// SPDX-License-Identifier: AGPL-3.0-only
import { type ReactNode, useId } from "react";
import { mergeProps, mergeRefs } from "react-aria";
import { tv } from "tailwind-variants";
import { useAnimationScope } from "./AnimationScope";
import { FeedArticle, type FeedArticleProps } from "./Feed";
import { AnimationsPlayContext } from "./Picture";
import { VisuallyHidden } from "./VisuallyHidden";

const postCard = tv({
  slots: {
    // The card is the size container its layout answers to (web-client/README.md D2).
    base: "@container relative mb-3 rounded-xl border border-border bg-surface-raised p-4 shadow-card",
    context:
      "mb-2 flex min-w-0 items-center gap-2 text-sm font-bold wrap-anywhere text-success [&>svg]:size-4 [&>svg]:shrink-0",
    grid: "grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-3 gap-y-2",
    // Narrow: the avatar sits beside the header and the body spans the card. Wide: the avatar has its own column.
    // Positioned, so that the avatar is drawn over the thread's line.
    avatar: "relative row-span-1 @md:row-span-5",
    // One row: the author takes the room the time leaves. Below `@sm` a PostCardAuthor puts its handle on a
    // second line and the author fills the row, so the time stays at the end of the name's line. A long time
    // (a full date) wraps inside at most half of the row instead of pushing the card wider.
    header: "flex min-w-0 items-baseline gap-x-2 self-center",
    author: "min-w-0 flex-1 wrap-anywhere @sm:flex-initial",
    time: "max-w-1/2 shrink-0 text-sm text-fg-muted",
    body: "col-span-2 min-w-0 @md:col-span-1 @md:col-start-2",
    details: "col-span-2 flex min-w-0 flex-wrap gap-x-3 gap-y-1 text-sm text-fg-muted @md:col-span-1 @md:col-start-2",
    reactions: "col-span-2 flex min-w-0 flex-wrap items-center gap-1 @md:col-span-1 @md:col-start-2",
    actions: "col-span-2 flex min-w-0 flex-wrap items-center gap-1 @md:col-span-1 @md:col-start-2",
    // The thread's line, down the middle of the avatar (1rem of padding and half of its 2.5rem), behind it.
    // It reaches from the card's top or from the avatar's middle (2.25rem down) to the card's bottom edge or
    // to the avatar's middle. It is decorative: the order of the posts and the headings around them carry
    // the thread.
    line: "pointer-events-none absolute inset-s-[calc(2.25rem-1px)] w-0.5 bg-border-strong",
  },
  variants: {
    // Told apart by a thicker border in the accent color, not by a color alone: it is the only card with a 2px border.
    highlighted: { true: { base: "border-2 border-accent" } },
    // Cards of a thread touch: the corners where they meet are square and the rule between them is the line's.
    lineAbove: {
      true: { base: "-mt-px rounded-t-none border-t-transparent", line: "-top-px" },
      false: { line: "top-9" },
    },
    // Below the avatar the line runs down the card, so on a narrow card the body, details and actions under
    // the header move clear of it; from `@md` they are in the avatar's next column anyway.
    lineBelow: {
      true: {
        base: "mb-0 rounded-b-none",
        line: "-bottom-px",
        body: "ps-8 @md:ps-0",
        details: "ps-8 @md:ps-0",
        reactions: "ps-8 @md:ps-0",
        actions: "ps-8 @md:ps-0",
      },
      false: { line: "h-9" },
    },
  },
});

export type PostCardProps = Omit<FeedArticleProps, "children" | "content" | "aria-labelledby" | "aria-describedby"> & {
  /**
   * Why the post is here, in one line above it ("Bob boosted"): a decorative icon (`aria-hidden`) and a text.
   * It starts the article's accessible name, before the author and time.
   */
  context?: ReactNode;
  /** The author's avatar, decorative because the name is next to it (`<Avatar decorative />`). */
  avatar?: ReactNode;
  /**
   * Who wrote it: usually a link around a PostCardAuthor (display name and handle). With `time`, it is the
   * article's accessible name. It shares one row with the time and takes the room the time leaves.
   */
  author: ReactNode;
  /** When it was posted: a `<time>` element, usually a link to the post. */
  time: ReactNode;
  /** The post body (PostContent), or a ContentWarning around it. It is the article's description. */
  content: ReactNode;
  /** The attachments (MediaGrid). */
  media?: ReactNode;
  /**
   * More about the post, between the body and the actions: items of one wrapping line of secondary text,
   * such as the full date and the counts in words of the post a page is about.
   */
  details?: ReactNode;
  /** The emoji reactions to the post, in one wrapping row above the actions. */
  reactions?: ReactNode;
  /** The actions on the post: reply, boost, favourite, the overflow menu. */
  actions?: ReactNode;
  /**
   * A name that starts the article's accessible name, before the context, author and time, for the card
   * that stands apart from the others ("Selected post"). It is not shown: `highlighted` shows the same.
   */
  label?: string;
  /** Whether the card stands apart from the cards around it, as the post a page is about does. */
  highlighted?: boolean;
  /** Whether the post continues a thread from the card above: a line from the card's top to the avatar. */
  lineAbove?: boolean;
  /** Whether the thread continues in the card below: a line from the avatar to the card's bottom. */
  lineBelow?: boolean;
};

/**
 * The layout of one post: an optional context line, avatar, author and time, body, media and actions in
 * slots, with the post's reactions in a row above the actions. The card knows nothing about posts; the caller fills the slots. It is an `<article>` labelled by
 * the context, author and time and described by the content, and inside a Feed it is one of the feed's
 * articles. The header is one row of the author and the time (with a PostCardAuthor, the handle goes to a
 * second line below the `@sm` container width). On a narrow card the body spans the full width under the
 * header; from the `@md` container width the avatar gets its own column.
 *
 * In a thread, `lineAbove` and `lineBelow` join the avatars of consecutive cards with a line (thread cards
 * have no context line, which would move the avatar down); on a narrow card with a line below, the rest
 * of the card is indented clear of it. The post a page is about is `highlighted`, with a `label` that
 * starts its name and its `details`.
 *
 * The card is an animation scope (`useAnimationScope`): while animations do not play by themselves, its avatar, emoji and
 * animated media move while it is hovered or has keyboard focus inside, and from a tap on it (not on a control inside it)
 * until another tap or until it scrolls off the screen.
 */
export function PostCard({
  context,
  avatar,
  author,
  time,
  content,
  media,
  details,
  reactions,
  actions,
  label,
  highlighted = false,
  lineAbove = false,
  lineBelow = false,
  className,
  ref,
  ...props
}: PostCardProps) {
  const slots = postCard({ highlighted, lineAbove, lineBelow });
  const scope = useAnimationScope<HTMLElement>();
  const id = useId();
  const labelId = `${id}-label`;
  const contextId = `${id}-context`;
  const authorId = `${id}-author`;
  const timeId = `${id}-time`;
  const contentId = `${id}-content`;
  const labelledBy = [
    ...(label === undefined ? [] : [labelId]),
    ...(context === undefined ? [] : [contextId]),
    authorId,
    timeId,
  ].join(" ");
  return (
    <FeedArticle
      {...mergeProps(props, scope.props)}
      ref={mergeRefs(ref, scope.props.ref)}
      aria-labelledby={labelledBy}
      aria-describedby={contentId}
      className={slots.base({ className })}
    >
      <AnimationsPlayContext value={scope.playing}>
        {label !== undefined && <VisuallyHidden id={labelId}>{label}</VisuallyHidden>}
        {(lineAbove || lineBelow) && <span aria-hidden="true" className={slots.line()} />}
        {context !== undefined && (
          <p id={contextId} className={slots.context()}>
            {context}
          </p>
        )}
        <div className={slots.grid()}>
          <div className={slots.avatar()}>{avatar}</div>
          <header className={slots.header()}>
            <div id={authorId} className={slots.author()}>
              {author}
            </div>
            <div id={timeId} className={slots.time()}>
              {time}
            </div>
          </header>
          <div id={contentId} className={slots.body()}>
            {content}
          </div>
          {media !== undefined && <div className={slots.body()}>{media}</div>}
          {details !== undefined && <div className={slots.details()}>{details}</div>}
          {reactions !== undefined && <div className={slots.reactions()}>{reactions}</div>}
          {actions !== undefined && <div className={slots.actions()}>{actions}</div>}
        </div>
      </AnimationsPlayContext>
    </FeedArticle>
  );
}

const postCardAuthor = tv({
  slots: {
    // Below `@sm` (of the card) the name and the handle are a line each; from `@sm` they share the header's row.
    // The lines start at the card's start, as wide as their text: a right-to-left name in a left-to-right
    // card lines up with the card, not with its own direction (G9; Chromium has no `text-align: match-parent`).
    base: "flex min-w-0 flex-col items-start @sm:flex-row @sm:items-baseline @sm:gap-1",
    name: "max-w-full min-w-0 truncate font-extrabold",
    // Left to right in any page, so the ellipsis at its end cuts the domain and the user part stays. The
    // handle is one text: elements inside would be blocks in the flex row and split it in the accessible name.
    handle: "max-w-full min-w-0 truncate text-sm text-fg-muted",
  },
});

export type PostCardAuthorProps = {
  /** The display name: the user's text in any script (`dir="auto"`). */
  name: ReactNode;
  /** The handle: `@user` for a local account, `@user@domain` for one of another server. */
  handle?: string;
};

/**
 * A display name and a handle for the author slot of PostCard, each cut to one line with an ellipsis
 * (web-client/16-look-states-and-wording.md G7). A handle is cut from its end, the domain's side. The text stays whole in the
 * DOM, so a link around it is named by the full name and handle. Below the card's `@sm` width the handle
 * is a line under the name; from it, both share the header's row with the time.
 */
export function PostCardAuthor({ name, handle }: PostCardAuthorProps) {
  const slots = postCardAuthor();
  return (
    <span className={slots.base()}>
      <span dir="auto" className={slots.name()}>
        {name}
      </span>
      {handle !== undefined && (
        <>
          {/* The space keeps the name and the handle apart in the accessible name. */}{" "}
          <span dir="ltr" className={slots.handle()}>
            {handle}
          </span>
        </>
      )}
    </span>
  );
}
