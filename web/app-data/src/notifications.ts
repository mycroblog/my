// SPDX-License-Identifier: AGPL-3.0-only
// The unread count of the notifications (ADR-0028 §18): the server holds it with the read marker, shared by every
// client. The cache reads it from GET /notifications/marker, adds each notification the stream receives, and reads it
// again after the stream broke (a reconnection or another tab taking the stream over).
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

type NotificationMarker = components["schemas"]["NotificationMarker"];

/** The read marker of the notifications and the number of notifications newer than it. */
export function notificationMarkerQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/notifications/marker");
}

/**
 * Counts a new notification that the stream received as unread. While the count is being read, the answer may or may
 * not count it, so the count is read again instead.
 */
export function receiveNotification(data: AppData): void {
  const { queryKey } = notificationMarkerQuery(data);
  if (data.queryClient.getQueryCache().find({ queryKey, exact: true })?.state.fetchStatus === "fetching") {
    receiveNotificationGap(data);
    return;
  }
  data.queryClient.setQueryData<NotificationMarker>(
    queryKey,
    (marker) => marker && { ...marker, unread_count: marker.unread_count + 1 },
  );
}

/** Reads the unread count again: notifications may have arrived while the stream was closed. */
export function receiveNotificationGap(data: AppData): void {
  void data.queryClient.invalidateQueries({ queryKey: notificationMarkerQuery(data).queryKey, exact: true });
}

/** The number of unread notifications of the viewer, for the shell; undefined until the server answered. */
export function useUnreadNotifications(): number | undefined {
  return useQuery(notificationMarkerQuery(useAppData())).data?.unread_count;
}
