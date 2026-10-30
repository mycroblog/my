// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements that show now (FR-726, ADR-0125 §4): those that every local user sees, with whether the
// viewer has read each, and marking one read, for the top of the home page below xl, the panel of the third column
// from xl, and the count of the navigation's item. The list loads with the page but is not part of it: while it loads
// and when it cannot be loaded, the page shows without it.
import { useMutation, useMutationState, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { shellQueryRetry } from "../../../lib/query";
import {
  type Announcement,
  announcementsQueryOptions,
  cacheAnnouncement,
  cachedAnnouncements,
  markAnnouncementReadMutation,
} from "../api/announcements";

export type Announcements = {
  /** The announcements, the latest first; empty while they load or when they cannot be loaded. */
  items: readonly Announcement[];
  /** How many of them the viewer has not read. */
  unread: number;
  /** Marks an announcement read; `onRead` runs once the list shows it read, with how many stay unread. */
  markRead: (announcement: Announcement, onRead: (unread: number) => void) => void;
  /** The IDs of the announcements being marked read. */
  markingIds: readonly string[];
  /** Why marking one read failed, localized. */
  error: string | undefined;
};

/** How many of `items` the viewer has not read. */
function unreadOf(items: readonly Announcement[]): number {
  return items.filter((item) => !item.read).length;
}

/** The announcements that show now. */
function useAnnouncementList(): readonly Announcement[] {
  const api = useApi();
  // Like the shell's queries: a 4xx is not tried again, anything else once, so a failure does not linger.
  const { data } = useQuery({ ...announcementsQueryOptions(api), retry: shellQueryRetry });
  return data?.items ?? [];
}

/** How many of the announcements that show now the viewer has not read, for the navigation's item. */
export function useUnreadAnnouncementCount(): number {
  return unreadOf(useAnnouncementList());
}

export function useAnnouncements(): Announcements {
  const api = useApi();
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | undefined>(undefined);
  const items = useAnnouncementList();
  const options = markAnnouncementReadMutation(api);
  const mark = useMutation({
    ...options,
    onMutate: () => {
      setError(undefined);
    },
    onSuccess: (announcement) => {
      cacheAnnouncement(queryClient, api, announcement);
    },
    onError: (failure) => {
      setError(errorMessage(failure));
    },
  });
  const markingIds = useMutationState({
    filters: { mutationKey: options.mutationKey, status: "pending" },
    select: (pending) => pending.state.variables as string,
  });
  return {
    items,
    unread: unreadOf(items),
    markRead: (announcement, onRead) => {
      mark.mutate(announcement.id, {
        onSuccess: () => {
          // The mutation's own onSuccess has written the answer into the list already.
          onRead(unreadOf(cachedAnnouncements(queryClient, api)));
        },
      });
    },
    markingIds,
    error,
  };
}
