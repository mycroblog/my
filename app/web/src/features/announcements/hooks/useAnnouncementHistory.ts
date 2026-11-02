// SPDX-License-Identifier: AGPL-3.0-only
// The list page of the announcements (/announcements, FR-726, ADR-0125 §3): those that have started to show, the
// ended ones included, the latest first, a page at a time, and marking one that shows now read.
import { useInfiniteQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useLoadFailure } from "../../../lib/loadFailure";
import { type Announcement, announcementHistoryQueryOptions } from "../api/announcements";
import { useMarkAnnouncementRead } from "./useMarkAnnouncementRead";

export type AnnouncementHistory =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      /** The announcements, the latest first. */
      items: readonly Announcement[];
      hasMore: boolean;
      loadMore: () => void;
      isLoadingMore: boolean;
      /** Why the next page could not be loaded, localized. */
      moreError: string | undefined;
      /** Marks an announcement read; `onRead` runs once the list shows it read. */
      markRead: (announcement: Announcement, onRead: () => void) => void;
      /** The IDs of the announcements being marked read. */
      markingIds: readonly string[];
      /** Why marking one read failed, localized. */
      markError: string | undefined;
    };

/** The announcements of the list page. */
export function useAnnouncementHistory(): AnnouncementHistory {
  const api = useApi();
  const mark = useMarkAnnouncementRead();
  // The route's loader has just tried the first page, and "Try again" is the user's own retry: no backoff behind it.
  const query = useInfiniteQuery({ ...announcementHistoryQueryOptions(api), retry: false, retryOnMount: false });
  const failure = useLoadFailure(query);
  const items = query.data?.pages.flatMap((page) => page.items) ?? [];
  if (query.data === undefined) {
    if (failure.error !== null) {
      return {
        status: "error",
        reason: errorMessage(failure.error),
        retry: () => void query.refetch(),
        isRetrying: failure.isRetrying,
      };
    }
    return { status: "loading" };
  }
  return {
    status: "ready",
    items,
    hasMore: query.hasNextPage,
    loadMore: () => {
      if (!query.isFetching) void query.fetchNextPage();
    },
    isLoadingMore: query.isFetchingNextPage,
    moreError: query.isFetchNextPageError ? errorMessage(query.error) : undefined,
    markRead: (announcement, onRead) => {
      mark.mutate(announcement.id, { onSuccess: onRead });
    },
    markingIds: mark.markingIds,
    markError: mark.error,
  };
}
