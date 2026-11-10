// SPDX-License-Identifier: AGPL-3.0-only
// The lists of events (ADR-0161 §9): the upcoming events that the user sees on the list page (`/events`, the nearest
// start first), the first of them in the third column ("Coming up"), and those an account organizes on its profile.
// Events are for the local users and need a session (read:events): a visitor who is not signed in has none of them,
// and a remote account organizes none.
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useLoadFailure } from "../../../lib/loadFailure";
import { shellQueryRetry } from "../../../lib/query";
import { type Event, eventRailQueryOptions, eventsQueryOptions } from "../api/events";

/** Whether the user has events: signed in. The entries to them (navigation, third column, profiles) show only then. */
function useEventsAvailable(): boolean {
  return useSessionStatus() === "signed-in";
}

/** The upcoming events as the list page shows them. */
export type EventList =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      events: readonly Event[];
      hasMore: boolean;
      loadMore: () => void;
      isLoadingMore: boolean;
      moreError: string | undefined;
    };

/** The upcoming events that the user sees, a page at a time, the nearest start first. */
export function useEventList(): EventList {
  const api = useApi();
  const query = useInfiniteQuery({ ...eventsQueryOptions(api), retry: shellQueryRetry, retryOnMount: false });
  const failure = useLoadFailure(query);
  if (query.data !== undefined) {
    return {
      status: "ready",
      events: query.data.pages.flatMap((page) => page.items),
      hasMore: query.hasNextPage,
      loadMore: () => {
        // A fetch in progress would be overwritten by the page fetched now (TanStack Query, "Infinite Queries").
        if (!query.isFetching) void query.fetchNextPage();
      },
      isLoadingMore: query.isFetchingNextPage,
      moreError: query.isFetchNextPageError ? errorMessage(query.error) : undefined,
    };
  }
  if (failure.error !== null) {
    return {
      status: "error",
      reason: errorMessage(failure.error),
      retry: () => void query.refetch(),
      isRetrying: failure.isRetrying,
    };
  }
  return { status: "loading" };
}

/**
 * The first upcoming events, for the third column, or the first that `organizerId` organizes, for its profile; empty
 * for a visitor who is not signed in, while they load, and when they fail (the page goes on without them).
 */
export function useEventRail(organizerId?: string, { enabled = true }: { enabled?: boolean } = {}): readonly Event[] {
  const api = useApi();
  const available = useEventsAvailable();
  const query = useQuery({
    ...eventRailQueryOptions(api, organizerId),
    enabled: available && enabled,
    retry: shellQueryRetry,
    retryOnMount: false,
  });
  return available && enabled ? (query.data?.items ?? []) : [];
}
