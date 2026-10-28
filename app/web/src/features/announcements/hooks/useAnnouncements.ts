// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements on the home page (FR-726): those that every local user sees now, with whether the
// viewer has read each, and marking one read. The list loads with the page but is not part of it: while it loads
// and when it cannot be loaded, the page shows the timeline alone.
import { useMutation, useMutationState, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { shellQueryRetry } from "../../../lib/query";
import {
  type Announcement,
  announcementsQueryOptions,
  cacheAnnouncement,
  markAnnouncementReadMutation,
} from "../api/announcements";

export type Announcements = {
  /** The announcements, the latest first; empty while they load or when they cannot be loaded. */
  items: readonly Announcement[];
  /** How many of them the viewer has not read. */
  unread: number;
  /** Marks an announcement read; `onRead` runs once the list shows it read. */
  markRead: (announcement: Announcement, onRead: () => void) => void;
  /** The IDs of the announcements being marked read. */
  markingIds: readonly string[];
  /** Why marking one read failed, localized. */
  error: string | undefined;
};

export function useAnnouncements(): Announcements {
  const api = useApi();
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | undefined>(undefined);
  // Like the shell's queries: a 4xx is not tried again, anything else once, so a failure does not linger.
  const { data } = useQuery({ ...announcementsQueryOptions(api), retry: shellQueryRetry });
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
  const items = data?.items ?? [];
  return {
    items,
    unread: items.filter((item) => !item.read).length,
    markRead: (announcement, onRead) => {
      mark.mutate(announcement.id, { onSuccess: onRead });
    },
    markingIds,
    error,
  };
}
