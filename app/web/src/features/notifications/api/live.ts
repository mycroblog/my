// SPDX-License-Identifier: AGPL-3.0-only
// What the streaming API's notifications change in the cache (web-client/README.md S10): the unread count of
// the read marker, which the navigation's badge shows, and the third column's panel, whose rows change as they come.
// The lists of the notifications page are only marked stale, not refetched (`refetchType: "none"`): the one on screen
// does not change under the reader, and the route fetches the marked ones again, and waits for them, when it opens
// next (web-client/README.md S7, ADR-0045).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import {
  type Notification,
  type NotificationMarker,
  type NotificationPage,
  notificationMarkerQueryOptions,
  notificationsPanelQueryOptions,
  panelLimit,
} from "./notifications";

/** The prefix of the keys of every filter of the list (notificationsQueryOptions). */
const listsKey = ["get", "/notifications"] as const;

/**
 * Puts a notification from the stream at the top of the panel, without a request: a burst of them reads nothing. A
 * panel with a read on its way, which may have missed it, or with no rows yet reads them again instead (only a panel on
 * screen does). A first read is not replaced by invalidating, which waits for it, so it starts over. Nothing is cached
 * while no timeline page has opened the panel.
 */
function addToPanel(queryClient: QueryClient, api: Api, notification: Notification): void {
  const { queryKey } = notificationsPanelQueryOptions(api);
  const state = queryClient.getQueryState(queryKey);
  if (state === undefined) return;
  if (state.data === undefined) {
    void queryClient.resetQueries({ queryKey, exact: true });
    return;
  }
  if (state.fetchStatus === "fetching") {
    void queryClient.invalidateQueries({ queryKey, exact: true });
    return;
  }
  queryClient.setQueryData<NotificationPage>(queryKey, (page) =>
    page === undefined
      ? page
      : {
          ...page,
          items: [notification, ...page.items.filter((item) => item.id !== notification.id)].slice(0, panelLimit),
        },
  );
}

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
  addToPanel(queryClient, api, notification);
  void queryClient.invalidateQueries({ queryKey: listsKey, refetchType: "none" });
}

/**
 * After the stream reconnected: reads the unread count and the panel on screen again, and the lists as a new
 * notification does.
 */
export function catchUpNotifications(queryClient: QueryClient, api: Api): void {
  void queryClient.invalidateQueries({ queryKey: notificationMarkerQueryOptions(api).queryKey, exact: true });
  void queryClient.invalidateQueries({ queryKey: notificationsPanelQueryOptions(api).queryKey, exact: true });
  void queryClient.invalidateQueries({ queryKey: listsKey, refetchType: "none" });
}
