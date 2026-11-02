// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { shellQueryReader, shellQueryRetry } from "../../../lib/query";
import { notificationMarkerQueryOptions, notificationsPanelQueryOptions } from "../api/notifications";
import { groupNotifications, type NotificationGroup } from "./groupNotifications";
import { isAfter } from "./useNotifications";

/** The rows of the third column's panel. */
export type NotificationsPanelList =
  /** The session is resuming, or the notifications are loading. */
  | { status: "loading" }
  /** They failed and none is loaded; `message` is localized. `isRetrying` while `retry` loads them again. */
  | { status: "error"; message: string; retry: () => void; isRetrying: boolean }
  | {
      status: "loaded";
      /** The latest notifications, with the favourites and the boosts of each post grouped, as on the page. */
      items: readonly NotificationGroup[];
      /** Whether a row is unread now: until the notifications page has shown it, since the panel moves no marker. */
      isUnread: (group: NotificationGroup) => boolean;
    };

/**
 * The latest notifications and the read marker, as they are in the cache: the stream keeps both up to date
 * (api/live.ts). Showing them marks nothing read; only the notifications page moves the marker.
 */
export function useNotificationsPanel(): NotificationsPanelList {
  const api = useApi();
  const enabled = useSessionStatus() === "signed-in";
  const list = useQuery({ ...notificationsPanelQueryOptions(api), enabled, retry: shellQueryRetry });
  const marker = useQuery({ ...notificationMarkerQueryOptions(api), ...shellQueryReader, enabled });
  if (list.data === undefined) {
    if (!list.isError) return { status: "loading" };
    return {
      status: "error",
      message: errorMessage(list.error),
      retry: () => {
        void list.refetch();
      },
      isRetrying: list.isFetching,
    };
  }
  return {
    status: "loaded",
    items: groupNotifications(list.data.items),
    isUnread: (group) => marker.data !== undefined && isAfter(group.notifications[0].id, marker.data),
  };
}
