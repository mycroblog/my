// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the voice chat rooms (web-client/README.md S1, ADR-0100 §8, §9, §13, §14): the open
// rooms that the user sees (GET /voice/rooms, also by hashtag), a few of them for the third column, one room
// (GET /voice/rooms/{id}) and its summary for the visitors who do not see it (GET /voice/rooms/{id}/summary), the
// room of an account's profile (GET /accounts/{id}), the rooms the user hosted (GET /voice/history), opening a room
// with its start post (POST /voice/rooms), its settings (PATCH /voice/rooms/{id}), and joining it
// (POST /voice/rooms/{id}/participation). Every voice operation answers 404 while the server has voice chat off.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { shellQueryRetry } from "../../../lib/query";

type Schemas = components["schemas"];

export type VoiceRoom = Schemas["VoiceRoom"];
export type VoiceAccount = Schemas["Account"];
export type VoiceRoomParticipant = Schemas["VoiceRoomParticipant"];
export type VoiceRoomSummary = Schemas["VoiceRoomSummary"];
export type VoiceRoomVisibility = Schemas["VoiceRoomVisibility"];
export type VoiceRoomSpeakers = Schemas["VoiceRoomSpeakers"];
export type VoiceRoomSettings = Schemas["VoiceRoomSettings"];
export type VoiceRoomUpdate = Schemas["VoiceRoomUpdate"];
export type VoiceRoomJoin = Schemas["VoiceRoomJoin"];
export type VoiceRoomEntry = Schemas["VoiceRoomEntry"];
export type VoiceRoomHistoryEntry = Schemas["VoiceRoomHistoryEntry"];

/** The visibilities of a room, in the order of the form. */
export const voiceRoomVisibilities = [
  "local",
  "followers",
  "mutuals",
  "invited",
] as const satisfies readonly VoiceRoomVisibility[];

/** Who may speak without asking, in the order of the form. */
export const voiceRoomSpeakers = [
  "everyone",
  "followers",
  "mutuals",
  "manual",
] as const satisfies readonly VoiceRoomSpeakers[];

/**
 * The limits of a room's settings, as the server checks them (internal/domain/voice, ADR-0100 回答 38): the name in
 * grapheme clusters (and in code points, `nameCodePoints`), the description and each hashtag in code points.
 */
export const voiceRoomLimits = {
  name: 100,
  nameCodePoints: 1000,
  description: 500,
  hashtags: 5,
  hashtagLength: 100,
} as const;

/** How many rooms the third column shows. */
const voiceRailSize = 3;

/** How many rooms one page of a list holds. */
const pageSize = 20;

/** The page of a room. */
export function voiceRoomPath(id: string): string {
  return `/voice/${id}`;
}

/** The instance, which says whether voice chat is on (`voice_enabled`) and who may open rooms (`voice_room_starters`). */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/** The first rooms of the list, for the third column. */
export function voiceRailQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/voice/rooms", { params: { query: { limit: voiceRailSize } } });
}

/** The open rooms that the user sees, those with `tag` only when given, a page at a time in the server's order. */
export function voiceRoomsQueryOptions(api: Api, tag?: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/voice/rooms", { tag }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = {
        limit: pageSize,
        ...(pageParam === undefined ? {} : { cursor: pageParam }),
        ...(tag === undefined ? {} : { tag }),
      };
      const { data, error } = await api.fetch.GET("/voice/rooms", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/**
 * An open room that the user sees; a 404 for one that ended or that the user does not see. A refusal (4xx) is not
 * tried again, so that the room's page says at once that the room ended; every reader of the room (the page, the call
 * bar) takes these options, so none of them puts the default three retries on the query they share.
 */
export function voiceRoomQueryOptions(api: Api, id: string) {
  return {
    ...api.query.queryOptions("get", "/voice/rooms/{id}", { params: { path: { id } } }),
    retry: shellQueryRetry,
  };
}

/** What the page of a room shows to a visitor who does not see it; needs no session. A 404 for an unknown room. */
export function voiceRoomSummaryQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/voice/rooms/{id}/summary", { params: { path: { id } } });
}

/** The account `id` with the open room where it hosts or speaks (`voice_room`), present only on this operation. */
export function accountVoiceRoomQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/accounts/{id}", { params: { path: { id } } });
}

/** The rooms the user hosted, newest first, a page at a time. */
export function voiceHistoryQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/voice/history", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = pageParam === undefined ? { limit: pageSize } : { limit: pageSize, cursor: pageParam };
      const { data, error } = await api.fetch.GET("/voice/history", { params: { query }, signal });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Opens a room that the user hosts, with its start post when given (POST /voice/rooms), as the options of useMutation. */
export function openVoiceRoomMutation(api: Api) {
  return {
    mutationKey: ["post", "/voice/rooms"] as const,
    mutationFn: async (settings: VoiceRoomSettings): Promise<VoiceRoomJoin> => {
      const { data, error } = await api.fetch.POST("/voice/rooms", { body: settings });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Changes the settings of a room that the user hosts (PATCH /voice/rooms/{id}), as the options of useMutation. */
export function updateVoiceRoomMutation(api: Api, id: string) {
  return {
    mutationKey: ["patch", "/voice/rooms/{id}", id] as const,
    mutationFn: async (update: VoiceRoomUpdate): Promise<VoiceRoom> => {
      const { data, error } = await api.fetch.PATCH("/voice/rooms/{id}", { params: { path: { id } }, body: update });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Joins a room as a listener with a new session (POST /voice/rooms/{id}/participation), as the options of useMutation. */
export function joinVoiceRoomMutation(api: Api, id: string) {
  return {
    mutationKey: ["post", "/voice/rooms/{id}/participation", id] as const,
    mutationFn: async (): Promise<VoiceRoomJoin> => {
      const { data, error } = await api.fetch.POST("/voice/rooms/{id}/participation", { params: { path: { id } } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Marks stale every list of rooms (the third column, the list, the hashtags' sections): their order and counts move. */
function refreshRoomLists(queryClient: QueryClient): Promise<void> {
  return queryClient.invalidateQueries({
    predicate: (query) => query.queryKey[0] === "get" && query.queryKey[1] === "/voice/rooms",
  });
}

/**
 * Puts a room as the server answered a write into its query (ADR-0045), and marks stale the lists, which it may enter,
 * leave, or move in, and the user's history after opening one.
 */
export async function cacheVoiceRoom(
  api: Api,
  queryClient: QueryClient,
  room: VoiceRoom,
  { opened = false }: { opened?: boolean } = {},
): Promise<void> {
  queryClient.setQueryData(voiceRoomQueryOptions(api, room.id).queryKey, room);
  await Promise.all([
    refreshRoomLists(queryClient),
    ...(opened ? [queryClient.invalidateQueries({ queryKey: ["get", "/voice/history"] })] : []),
  ]);
}
