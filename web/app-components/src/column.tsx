// SPDX-License-Identifier: AGPL-3.0-only

// A timeline's column: its posts one under another in a feed of @my/design-system, a space apart on the page's one
// surface, with the feed's keys and its scroll of a post the keyboard focuses to the top. It shows that the first posts
// load, that they failed (with a retry), or that there are none (the feature's `empty`, else a plain line), and under
// the posts that more load or failed to (with a retry); near the end it asks for more. It fetches nothing: it takes a timeline in the shape of @my/app-data's (`useHomeTimeline()`), which a
// feature passes on as it is, and each post's action goes to `onAction`. `moveFocus` moves the focus between the posts
// of the column it is in, which the column's shortcuts of the next and the previous post use. The feed keeps the
// reader's place as posts go in above; when they go in, and what the new posts' button says, is the feature's.
// `ListColumn` is the same column of any list (the notifications, a profile's posts), each item drawn by its feature.

import {
  Feed,
  FeedItem,
  FeedNotice,
  moveFeedFocus,
  TextButton,
  useAction,
  useLocale,
  type FeedProps,
  type NewItems,
} from "@my/design-system";
import { stringsIn } from "./locale";
import { Post, type PostAction, type PostProps } from "./post";
import type { PostStatus } from "./status";
import type { ReactNode } from "react";

/** A timeline as the column reads it: the shape of what @my/app-data's timeline hooks return. */
export type ColumnTimeline = {
  /** The statuses fetched so far, newest first. */
  readonly statuses: readonly PostStatus[];
  /** `pending` until the first page arrives, `error` when it failed, then `success`. */
  readonly status: "pending" | "error" | "success";
  /** The failure of the last fetch, or null. */
  readonly error: Error | null;
  readonly hasNextPage: boolean;
  readonly isFetchingNextPage: boolean;
  /** Fetches the next page; nothing while one is being fetched or when there is none. */
  readonly fetchNextPage: () => void;
  /** Fetches the timeline again from its first page, after the first page failed. */
  readonly refetch: () => void;
};

export type ColumnProps = ColumnTimeline & {
  /** The timeline's name, for assistive technology. */
  label: string;
  /** What the reader asked of a post. */
  onAction?: ((action: PostAction) => void) | undefined;
  /** The rest of a post's props, by its status: whether the reader wrote it, its translation. */
  postProps?: ((status: PostStatus) => Omit<PostProps, "status" | "onAction">) | undefined;
  /** What the column says when the timeline has no posts, with the feature's next action; a plain line by default. */
  empty?: ReactNode;
  /** Hears whether the column's top is in view, when that changes. */
  onTopChange?: ((atTop: boolean) => void) | undefined;
  /** Hears whether the reader is in the column (focus or a selection inside it), when that changes. */
  onReaderChange?: ((inside: boolean) => void) | undefined;
  /** The new posts' button over the column's top, which the feature words; left out while it has nothing to tell. */
  newPosts?: NewItems | undefined;
};

/**
 * Moves the focus from `from` (a post of a column, or something inside one) to the next post (1) or the previous one
 * (-1), and scrolls that post to the top of the view; returns whether it moved.
 */
export const moveFocus = (from: Element | null, step: 1 | -1, options: { repeat?: boolean } = {}): boolean =>
  moveFeedFocus(from, step, options);

/**
 * The keys of the next and the previous post (timeline.next, timeline.previous), while focus is on a post or inside
 * one; at either end the key goes on to the page. A widget inside a post that takes the same key (a menu's arrows)
 * keeps it. A feed a feature lays out itself (a thread) places them as `FeedKeys`.
 */
function Moves() {
  const { t } = stringsIn(useLocale().locale);
  const groupLabel = t("column.shortcuts");
  useAction("timeline.next", ({ repeat }) => moveFocus(document.activeElement, 1, { repeat }), {
    scope: "focus",
    label: t("column.next"),
    groupLabel,
  });
  useAction("timeline.previous", ({ repeat }) => moveFocus(document.activeElement, -1, { repeat }), {
    scope: "focus",
    label: t("column.previous"),
    groupLabel,
  });
  return null;
}

export { Moves as FeedKeys };

/** What a list's column says while it loads, when it failed, and when it is empty, in place of the posts' words. */
export type ListWords = {
  readonly loading: string;
  readonly loadingMore: string;
  readonly failed: string;
  readonly failedMore: string;
  readonly empty: string;
};

export type ListColumnProps<T> = Omit<ColumnTimeline, "statuses"> & {
  /** The list's name, for assistive technology. */
  label: string;
  /** The items fetched so far, in the list's order. */
  items: readonly T[];
  /** An item's key, unique in the list: what it shows (a post's id). */
  keyOf: (item: T) => string;
  /** An item's entry. */
  children: (item: T) => ReactNode;
  /** What the column says while it loads, fails, or has nothing; the posts' words by default. */
  words?: ListWords | undefined;
  /** What the column says when the list is empty, with the feature's next action; a plain line of `words` by default. */
  empty?: ReactNode;
} & Pick<FeedProps, "onTopChange" | "onReaderChange" | "newItems" | "onPassed" | "onEntryClick">;

/**
 * A list's column: its entries in a feed, as a timeline's column shows its posts, with the same notices of loading,
 * failures and an empty list, in the list's own words.
 */
export function ListColumn<T>({
  label,
  items,
  keyOf,
  children,
  status,
  error,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  refetch,
  words,
  empty,
  ...feed
}: ListColumnProps<T>) {
  const { t } = stringsIn(useLocale().locale);
  const say = words ?? {
    loading: t("column.loading"),
    loadingMore: t("column.loading_more"),
    failed: t("column.failed"),
    failedMore: t("column.failed_more"),
    empty: t("column.empty"),
  };
  const failedMore = status === "success" && error !== null && !isFetchingNextPage;
  const retry = (onPress: () => void) => <TextButton onPress={onPress}>{t("column.retry")}</TextButton>;
  const end =
    status === "pending" ? (
      <FeedNotice>{say.loading}</FeedNotice>
    ) : status === "error" ? (
      <FeedNotice alert action={retry(refetch)}>
        {say.failed}
      </FeedNotice>
    ) : isFetchingNextPage ? (
      <FeedNotice>{say.loadingMore}</FeedNotice>
    ) : failedMore && hasNextPage ? (
      <FeedNotice alert action={retry(fetchNextPage)}>
        {say.failedMore}
      </FeedNotice>
    ) : items.length === 0 ? (
      (empty ?? <FeedNotice>{say.empty}</FeedNotice>)
    ) : undefined;
  // After a failure it asks again only when the reader tries again, never by itself.
  const asks = status === "success" && hasNextPage && !failedMore;
  return (
    <Feed
      label={label}
      count={items.length}
      more={status !== "success" || hasNextPage}
      busy={status === "pending" || isFetchingNextPage}
      onNearEnd={asks ? fetchNextPage : undefined}
      end={end}
      {...feed}
    >
      <Moves />
      {items.map((item, i) => {
        const key = keyOf(item);
        return (
          <FeedItem key={key} id={key} position={i + 1}>
            {children(item)}
          </FeedItem>
        );
      })}
    </Feed>
  );
}

export function Column({ statuses, onAction, postProps, newPosts, ...rest }: ColumnProps) {
  return (
    <ListColumn {...rest} items={statuses} keyOf={(s) => s.id} newItems={newPosts}>
      {(s) => <Post {...postProps?.(s)} status={s} onAction={onAction} />}
    </ListColumn>
  );
}
