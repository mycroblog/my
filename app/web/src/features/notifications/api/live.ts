// SPDX-License-Identifier: AGPL-3.0-only
// What the streaming API's notifications change in the cache (web-client/README.md S10): the unread count of
// the read marker, which the navigation's badge shows. The lists are only marked stale, not refetched
// (`refetchType: "none"`): the one on screen does not change under the reader, and the route fetches the
// marked ones again, and waits for them, when it opens next (web-client/README.md S7, ADR-0045).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { type Notification, type NotificationMarker, notificationMarkerQueryOptions } from "./notifications";

/** The prefix of the keys of every filter of the list (notificationsQueryOptions). */
const listsKey = ["get", "/notifications"] as const;

/** A new notification from the stream: one more unread, when it comes after the read marker. */
export function receiveNotification(queryClient: QueryClient, api: Api, notification: Notification): void {
  const markerKey = notificationMarkerQueryOptions(api).queryKey;
  if (queryClient.getQueryData(markerKey) === undefined) {
    void queryClient.invalidateQueries({ queryKey: markerKey, exact: true });
  } else {
    queryClient.setQueryData<NotificationMarker>(markerKey, (marker) => {
      // IDs are UUIDv7 strings, which sort in the order they were made.
      const unread =
        marker !== undefined && (marker.last_read_id === undefined || notification.id > marker.last_read_id);
      return unread ? { ...marker, unread_count: marker.unread_count + 1 } : marker;
    });
  }
  void queryClient.invalidateQueries({ queryKey: listsKey, refetchType: "none" });
}

/** After the stream reconnected: reads the unread count again, and the lists as a new notification does. */
export function catchUpNotifications(queryClient: QueryClient, api: Api): void {
  void queryClient.invalidateQueries({ queryKey: notificationMarkerQueryOptions(api).queryKey, exact: true });
  void queryClient.invalidateQueries({ queryKey: listsKey, refetchType: "none" });
}
