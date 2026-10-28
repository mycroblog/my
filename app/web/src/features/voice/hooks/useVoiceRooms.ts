// SPDX-License-Identifier: AGPL-3.0-only
// Whether the server has voice chat on and who may open rooms (GET /instance, ADR-0100 §2), and the open rooms that
// the user sees: a few in the third column, all of them on the list page, those of a hashtag on its page (§8). Voice
// chat needs a session (read:voice): a visitor who is not signed in has none of it.
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { shellQueryReader, shellQueryRetry } from "../../../lib/query";
import { useViewerPermissions } from "../../moderation";
import { instanceQueryOptions, type VoiceRoom, voiceRailQueryOptions, voiceRoomsQueryOptions } from "../api/voice";

/**
 * Whether the signed-in user has voice chat: the server has it on (`voice_enabled`). False while the instance loads
 * and for a visitor who is not signed in. The entries to voice chat (navigation, third column, hashtags, profiles,
 * settings) show only then.
 */
export function useVoiceAvailable(): boolean {
  const api = useApi();
  const signedIn = useSessionStatus() === "signed-in";
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  return signedIn && instance.data?.voice_enabled === true;
}

/**
 * Whether the user may open a room: voice chat is on, and the server lets everyone open rooms or the user's roles have
 * `start_voice_rooms` (`voice_room_starters`). The server still refuses one that may not (a 403), which the form says.
 */
export function useCanStartVoiceRoom(): boolean {
  const api = useApi();
  const available = useVoiceAvailable();
  const permissions = useViewerPermissions();
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  return available && (instance.data?.voice_room_starters === "everyone" || permissions.includes("start_voice_rooms"));
}

/** The first rooms of the list for the third column, while voice chat is on; empty otherwise and while they load. */
export function useVoiceRail(): readonly VoiceRoom[] {
  const api = useApi();
  const available = useVoiceAvailable();
  const query = useQuery({
    ...voiceRailQueryOptions(api),
    enabled: available,
    retry: shellQueryRetry,
    retryOnMount: false,
  });
  return available ? (query.data?.items ?? []) : [];
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
 * follows the host or a speaker first, then by the most participants. `enabled` false loads nothing (`loading`). The
 * list page asks even before it knows whether voice chat is on, since it is opened by its address too; a 404 says off.
 */
export function useVoiceRooms({ tag, enabled = true }: { tag?: string; enabled?: boolean } = {}): VoiceRoomList {
  const api = useApi();
  const query = useInfiniteQuery({
    ...voiceRoomsQueryOptions(api, tag),
    enabled,
    retry: shellQueryRetry,
    retryOnMount: false,
  });
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
    return {
      status: "error",
      reason: errorMessage(query.error),
      retry: () => void query.refetch(),
      isRetrying: query.isFetching,
    };
  }
  return { status: "loading" };
}

/** The open rooms with `tag`, for the top of the hashtag's page; empty while voice chat is off or nothing is open. */
export function useTagVoiceRooms(tag: string): readonly VoiceRoom[] {
  const available = useVoiceAvailable();
  const list = useVoiceRooms({ tag, enabled: available });
  return available && list.status === "ready" ? list.rooms : [];
}
