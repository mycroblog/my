// SPDX-License-Identifier: AGPL-3.0-only
// Whether the server has voice chat on, and the open rooms that the user sees: a few in the third column, all of them
// on the list page, those of a hashtag on its page (ADR-0100 §2, §8). Voice chat needs a session (read:voice): a
// visitor who is not signed in has none of it.
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { shellQueryRetry } from "../../../lib/query";
import { type VoiceRoom, voiceRailQueryOptions, voiceRoomsQueryOptions } from "../api/voice";

/**
 * The first rooms of the list, which also tell whether voice chat is on: `available` once the server answered the
 * list, false while it loads, for a visitor who is not signed in, and when it answered 404 (voice chat off). The
 * shell reads it on every page, so a refusal is not tried again (shellQueryRetry).
 */
export function useVoiceRail(): { available: boolean; rooms: readonly VoiceRoom[] } {
  const api = useApi();
  const signedIn = useSessionStatus() === "signed-in";
  const query = useQuery({ ...voiceRailQueryOptions(api), enabled: signedIn, retry: shellQueryRetry, retryOnMount: false });
  if (!signedIn || query.data === undefined) return { available: false, rooms: [] };
  return { available: true, rooms: query.data.items };
}

/** Whether the server has voice chat on for the signed-in user, for the entries to it (navigation, settings, profiles). */
export function useVoiceAvailable(): boolean {
  return useVoiceRail().available;
}

/** The open rooms as a list shows them. `unavailable` is the server with voice chat off (404). */
export type VoiceRoomList =
  | { status: "unavailable" }
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      rooms: readonly VoiceRoom[];
      hasMore: boolean;
      loadMore: () => void;
      isLoadingMore: boolean;
      moreError: string | undefined;
    };

/**
 * The open rooms that the user sees (with `tag`, those of the hashtag), in the server's order: the rooms where the user
 * follows the host or a speaker first, then by the most participants. `enabled` false loads nothing (`loading`).
 */
export function useVoiceRooms({ tag, enabled = true }: { tag?: string; enabled?: boolean } = {}): VoiceRoomList {
  const api = useApi();
  const query = useInfiniteQuery({ ...voiceRoomsQueryOptions(api, tag), enabled, retry: shellQueryRetry, retryOnMount: false });
  if (query.data !== undefined) {
    return {
      status: "ready",
      rooms: query.data.pages.flatMap((page) => page.items),
      hasMore: query.hasNextPage,
      loadMore: () => {
        // A fetch in progress would be overwritten by the page fetched now (TanStack Query, "Infinite Queries").
        if (!query.isFetching) void query.fetchNextPage();
      },
      isLoadingMore: query.isFetchingNextPage,
      moreError: query.isFetchNextPageError ? errorMessage(query.error) : undefined,
    };
  }
  if (query.error !== null) {
    if (query.error instanceof ApiError && query.error.status === 404) return { status: "unavailable" };
    return { status: "error", reason: errorMessage(query.error), retry: () => void query.refetch(), isRetrying: query.isFetching };
  }
  return { status: "loading" };
}

/** The open rooms with `tag`, for the top of the hashtag's page; empty while voice chat is off or nothing is open. */
export function useTagVoiceRooms(tag: string): readonly VoiceRoom[] {
  const available = useVoiceAvailable();
  const list = useVoiceRooms({ tag, enabled: available });
  return available && list.status === "ready" ? list.rooms : [];
}
