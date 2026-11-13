// SPDX-License-Identifier: AGPL-3.0-only

// A timeline's column: its posts one under another in a feed of @my/design-system, a space apart on the page's one
// surface, with the feed's keys and its scroll of a post the keyboard focuses to the top. It shows that the first posts
// load, that they failed (with a retry), or that there are none (the feature's `empty`, else a plain line), and under
// the posts that more load or failed to (with a retry); near the end it asks for more. It fetches nothing: it takes a timeline in the shape of @my/app-data's (`useHomeTimeline()`), which a
// feature passes on as it is, and each post's action goes to `onAction`. `moveFocus` moves the focus between the posts
// of the column it is in, for the shortcuts of the next and the previous post.

import { Feed, FeedItem, FeedNotice, moveFeedFocus, TextButton, useLocale } from "@my/design-system";
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
};

/**
 * Moves the focus from `from` (a post of a column, or something inside one) to the next post (1) or the previous one
 * (-1), and scrolls that post to the top of the view; returns whether it moved.
 */
export const moveFocus = (from: Element | null, step: 1 | -1): boolean => moveFeedFocus(from, step);

export function Column({
  label,
  statuses,
  status,
  error,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  refetch,
  onAction,
  postProps,
  empty,
}: ColumnProps) {
  const { t } = stringsIn(useLocale().locale);
  const failedMore = status === "success" && error !== null && !isFetchingNextPage;
  const retry = (onPress: () => void) => <TextButton onPress={onPress}>{t("column.retry")}</TextButton>;
  const end =
    status === "pending" ? (
      <FeedNotice>{t("column.loading")}</FeedNotice>
    ) : status === "error" ? (
      <FeedNotice alert action={retry(refetch)}>
        {t("column.failed")}
      </FeedNotice>
    ) : isFetchingNextPage ? (
      <FeedNotice>{t("column.loading_more")}</FeedNotice>
    ) : failedMore && hasNextPage ? (
      <FeedNotice alert action={retry(fetchNextPage)}>
        {t("column.failed_more")}
      </FeedNotice>
    ) : statuses.length === 0 ? (
      (empty ?? <FeedNotice>{t("column.empty")}</FeedNotice>)
    ) : undefined;
  // After a failure it asks again only when the reader tries again, never by itself.
  const asks = status === "success" && hasNextPage && !failedMore;
  return (
    <Feed
      label={label}
      count={statuses.length}
      more={status !== "success" || hasNextPage}
      busy={status === "pending" || isFetchingNextPage}
      onNearEnd={asks ? fetchNextPage : undefined}
      end={end}
    >
      {statuses.map((s, i) => (
        <FeedItem key={s.id} position={i + 1}>
          <Post {...postProps?.(s)} status={s} onAction={onAction} />
        </FeedItem>
      ))}
    </Feed>
  );
}
