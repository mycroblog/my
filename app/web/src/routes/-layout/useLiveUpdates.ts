// SPDX-License-Identifier: AGPL-3.0-only
// The layout keeps the one event stream of the app open (web-client/README.md S10, FR-207) and routes its events
// to the features that own the cached data: new home entries wait behind the home timeline's "N new
// posts" button (or join its cache while no page shows it), and so do the new entries of a loaded list's timeline
// (`list_update`, ADR-0122 §4); notifications update the unread badge and play the
// notification sound where this device has it on for their type (lib/notificationSound), and a moderator's notifications of a new report or
// registration read the Moderation entry's counts again, and the server's announcements join, change, or leave
// the list above the home timeline. After a reconnect, all of them read again what the
// gap may have missed, since the server replays nothing (client-api.md §5).
import { useQueryClient } from "@tanstack/react-query";
import { catchUpAnnouncements, receiveAnnouncement, removeAnnouncement } from "../../features/announcements";
import { refreshAdminCounts } from "../../features/moderation";
import { catchUpNotifications, receiveNotification } from "../../features/notifications";
import { catchUpHome, catchUpLists, receiveHomeStatus, receiveListStatus } from "../../features/timeline";
import { useApi } from "../../lib/auth/SessionProvider";
import { playNotificationSound } from "../../lib/notificationSound";
import { useEventStream } from "../../lib/sse/useEventStream";

export function useLiveUpdates(): void {
  const api = useApi();
  const queryClient = useQueryClient();
  useEventStream({
    onEvent: (event) => {
      switch (event.type) {
        case "update":
          receiveHomeStatus(queryClient, api, event.status);
          break;
        case "list_update":
          receiveListStatus(queryClient, api, event.listId, event.status);
          break;
        case "notification":
          receiveNotification(queryClient, api, event.notification);
          refreshAdminCounts(queryClient, api, event.notification.type);
          playNotificationSound(event.notification.type);
          break;
        case "announcement":
          receiveAnnouncement(queryClient, api, event.announcement);
          break;
        case "announcement_removed":
          removeAnnouncement(queryClient, api, event.id);
          break;
      }
    },
    onReconnect: () => {
      catchUpHome(queryClient, api).catch((error: unknown) => {
        // The next reconnect or a visit to the timeline reads it again.
        console.warn("the home timeline could not be read after reconnecting", error);
      });
      catchUpLists(queryClient, api).catch((error: unknown) => {
        // The next reconnect or a visit to the list reads it again.
        console.warn("a list timeline could not be read after reconnecting", error);
      });
      catchUpNotifications(queryClient, api);
      catchUpAnnouncements(queryClient, api);
      refreshAdminCounts(queryClient, api);
    },
  });
}
