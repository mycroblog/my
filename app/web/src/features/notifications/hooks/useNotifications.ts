// SPDX-License-Identifier: AGPL-3.0-only
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useLoadFailure } from "../../../lib/loadFailure";
import {
  cacheReadMarker,
  type NotificationFilter,
  type NotificationMarker,
  notificationMarkerQueryOptions,
  notificationsQueryOptions,
  updateNotificationMarker,
} from "../api/notifications";
import { groupNotifications, type NotificationGroup } from "./groupNotifications";
import { shellQueryRetry } from "../../../lib/query";

/** The notifications page's list. */
export type NotificationList =
  | { status: "signed-out" }
  /** The session is resuming, or the first page is loading. */
  | { status: "loading" }
  /** The first page failed; `message` is localized. `isRetrying` while `retry` loads it again. */
  | { status: "error"; message: string; retry: () => void; isRetrying: boolean }
  | {
      status: "loaded";
      /** The rows: the loaded notifications, with the favourites and the boosts of each post grouped. */
      items: readonly NotificationGroup[];
      hasMore: boolean;
      isLoadingMore: boolean;
      loadMore: () => void;
      /**
       * Whether a row was unread when the page opened, so it stays marked as new while the page is open: a
       * group is unread when its newest notification is.
       */
      isUnread: (group: NotificationGroup) => boolean;
    };

/** Whether `id` comes after the marker: IDs are UUIDv7 strings, which sort in the order they were made. */
export function isAfter(id: string, marker: NotificationMarker): boolean {
  return marker.last_read_id === undefined || id > marker.last_read_id;
}

/**
 * The read marker as it was when the page opened: the page keeps showing which notifications were new
 * after it has marked them read. It is a snapshot taken once (useState's initializer), not a copy kept in
 * sync with the server (web-client/README.md S2); `null` when the marker had not loaded, and nothing is marked new.
 */
function useMarkerAtOpen(marker: NotificationMarker | undefined): NotificationMarker | null {
  const [atOpen] = useState(() => marker ?? null);
  return atOpen;
}

/**
 * Moves the read marker to `newestId`, the newest notification on screen, whenever a newer one is shown.
 * It is an Effect because the reason is that the notifications were displayed, not something the user
 * did (react.dev, "You Might Not Need an Effect": sending a POST because a component was displayed).
 * The server moves the marker only forward, so a repeated or late request changes nothing; a failure is
 * left alone, and the next visit sends it again.
 */
function useMarkRead(newestId: string | undefined, marker: NotificationMarker | undefined) {
  const api = useApi();
  const queryClient = useQueryClient();
  const { mutate } = useMutation({
    mutationFn: (lastReadId: string) => updateNotificationMarker(api, lastReadId),
    onSuccess: (updated) => {
      cacheReadMarker(api, queryClient, updated);
    },
  });
  /** The ID last sent, so that a re-run of the Effect (Strict Mode, a refetch) does not send it again. */
  const sentRef = useRef<string | null>(null);
  const shouldSend = newestId !== undefined && marker !== undefined && isAfter(newestId, marker);
  useEffect(() => {
    if (!shouldSend || sentRef.current === newestId) return;
    sentRef.current = newestId;
    mutate(newestId);
  }, [shouldSend, newestId, mutate]);
}

/**
 * The notifications under `filter`, and the read marker. While the page shows every type it marks the
 * newest notification read; a filtered list does not, since the marker is one position over all types and
 * would pass notifications the filter hides.
 */
export function useNotifications(filter: NotificationFilter): NotificationList {
  const api = useApi();
  const session = useSessionStatus();
  const signedIn = session === "signed-in";
  // The route's loader has just tried the first page; a failure stays until the retry button or the return
  // online (lib/query.ts) instead of being tried again as the list mounts.
  const list = useInfiniteQuery({
    ...notificationsQueryOptions(api, filter),
    enabled: signedIn,
    retryOnMount: false,
    retry: shellQueryRetry,
  });
  const failure = useLoadFailure(list);
  const marker = useQuery({ ...notificationMarkerQueryOptions(api), enabled: signedIn });
  const markerAtOpen = useMarkerAtOpen(marker.data);
  const notifications = list.data?.pages.flatMap((page) => page.items) ?? [];
  // The marker is a position among the notifications, not the rows: it moves to the newest one.
  useMarkRead(signedIn && filter === "all" ? notifications[0]?.id : undefined, marker.data);

  if (session === "signed-out") return { status: "signed-out" };
  if (failure.error !== null && list.data === undefined) {
    return {
      status: "error",
      message: errorMessage(failure.error),
      retry: () => {
        void list.refetch();
      },
      isRetrying: failure.isRetrying,
    };
  }
  if (list.data === undefined) return { status: "loading" };
  return {
    status: "loaded",
    items: groupNotifications(notifications),
    hasMore: list.hasNextPage,
    isLoadingMore: list.isFetchingNextPage,
    loadMore: () => {
      if (!list.isFetchingNextPage) void list.fetchNextPage();
    },
    isUnread: (group) => markerAtOpen !== null && isAfter(group.notifications[0].id, markerAtOpen),
  };
}
