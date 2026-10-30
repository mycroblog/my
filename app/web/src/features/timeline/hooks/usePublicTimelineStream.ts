// SPDX-License-Identifier: AGPL-3.0-only
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useEventStream } from "../../../lib/sse/useEventStream";
import { catchUpPublicTimeline, type PublicStreamedTimeline, receivePublicTimelineStatus } from "../api/newStatuses";

/**
 * Keeps the stream of the local or the federated timeline open while its screen is shown and a user is signed in
 * (ADR-0127 §5): its new posts wait for the reader like the home timeline's (newStatuses.ts), and after a reconnect the
 * timeline's first page is read again, since the server replays nothing. Leaving the screen closes the stream.
 */
export function usePublicTimelineStream(timeline: PublicStreamedTimeline): void {
  const api = useApi();
  const queryClient = useQueryClient();
  useEventStream({
    timeline: timeline.kind === "local" ? "local" : "federated",
    onEvent: (event) => {
      if (event.type === "update") receivePublicTimelineStatus(queryClient, api, timeline, event.status);
    },
    onReconnect: () => {
      catchUpPublicTimeline(queryClient, api, timeline).catch((error: unknown) => {
        // The next reconnect or a visit to the timeline reads it again.
        console.warn("the timeline could not be read after reconnecting", error);
      });
    },
  });
}
