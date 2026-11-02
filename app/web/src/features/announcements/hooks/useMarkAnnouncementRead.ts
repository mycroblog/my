// SPDX-License-Identifier: AGPL-3.0-only
// Marking an announcement read, for every place that has "Mark as read": the answer goes into the cache, and the
// failure and the IDs being marked are the place's to show.
import { useMutation, useMutationState, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { cacheAnnouncement, markAnnouncementReadMutation } from "../api/announcements";

export function useMarkAnnouncementRead() {
  const api = useApi();
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | undefined>(undefined);
  const options = markAnnouncementReadMutation(api);
  const { mutate } = useMutation({
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
  return { mutate, markingIds, error };
}
