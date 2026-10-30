// SPDX-License-Identifier: AGPL-3.0-only
// The list page of the announcements (/announcements, FR-726, ADR-0125 §3): those that have started to show, the
// ended ones included, the latest first, a page at a time, and marking one that shows now read. Opened for one
// announcement (from its notification), the list loads pages until it has it and marks it read; when no page has it,
// it was withdrawn.
import { useInfiniteQuery, useMutation, useMutationState, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import {
  type Announcement,
  announcementHistoryQueryOptions,
  cacheAnnouncement,
  markAnnouncementReadMutation,
} from "../api/announcements";

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
      /**
       * The announcement the page was opened for: `found` once a page has it, `missing` once no page has it (it was
       * withdrawn), `searching` while more pages load, and `none` without one.
       */
      target: "none" | "searching" | "found" | "missing";
    };

/** The announcements of the list page; `targetId` is the one the page was opened for, if any. */
export function useAnnouncementHistory(targetId: string | undefined): AnnouncementHistory {
  const api = useApi();
  const queryClient = useQueryClient();
  const [markError, setMarkError] = useState<string | undefined>(undefined);
  const query = useInfiniteQuery(announcementHistoryQueryOptions(api));
  const options = markAnnouncementReadMutation(api);
  const mark = useMutation({
    ...options,
    onMutate: () => {
      setMarkError(undefined);
    },
    onSuccess: (announcement) => {
      cacheAnnouncement(queryClient, api, announcement);
    },
    onError: (failure) => {
      setMarkError(errorMessage(failure));
    },
  });
  const markingIds = useMutationState({
    filters: { mutationKey: options.mutationKey, status: "pending" },
    select: (pending) => pending.state.variables as string,
  });
  const items = query.data?.pages.flatMap((page) => page.items) ?? [];
  const target = items.find((item) => item.id === targetId);
  const { isFetching, fetchNextPage } = query;
  const loadsNext = targetId !== undefined && target === undefined && query.hasNextPage && !query.isFetchNextPageError;
  // While the cached pages are read again (an announcement newer than them), they may still get it.
  const searching = targetId !== undefined && target === undefined && (loadsNext || isFetching);
  // Opened for an announcement that the first pages do not have: the next ones load until one has it (TanStack Query,
  // "Infinite Queries": a fetch in progress would be overwritten, so only between fetches).
  useEffect(() => {
    if (loadsNext && !isFetching) void fetchNextPage();
  }, [loadsNext, isFetching, fetchNextPage]);
  // Opening an announcement from its notification marks it read (ADR-0125 §2), once.
  const openedRef = useRef<string | undefined>(undefined);
  const { mutate } = mark;
  useEffect(() => {
    if (target === undefined || openedRef.current === target.id) return;
    openedRef.current = target.id;
    if (!target.read && !target.ended) mutate(target.id);
  }, [target, mutate]);
  if (query.data === undefined) {
    if (query.error !== null) {
      return {
        status: "error",
        reason: errorMessage(query.error),
        retry: () => void query.refetch(),
        isRetrying: query.isFetching,
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
    markingIds,
    markError,
    target: targetId === undefined ? "none" : target !== undefined ? "found" : searching ? "searching" : "missing",
  };
}
