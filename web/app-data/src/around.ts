// SPDX-License-Identifier: AGPL-3.0-only
// What is around the reader beside the timeline (ADR-0100 §8, FR-928, FR-951): the first open voice rooms the viewer
// sees, in the server's order (the rooms of whom they follow first, then the fullest), and the first upcoming events,
// the nearest first; each with whether there are more than the column shows.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** An open voice room. */
export type VoiceRoom = components["schemas"]["VoiceRoom"];

/** An event of the server's people. */
export type ServerEvent = components["schemas"]["Event"];

/** The first items of a list, and whether it holds more. */
export type FirstOf<T> = { readonly items: readonly T[]; readonly more: boolean };

/** How many rooms and events the column beside the timeline shows. */
export const aroundLimit = 3;

/** The first open voice rooms the viewer sees. */
export function openVoiceRoomsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/voice/rooms", { params: { query: { limit: aroundLimit } } });
}

/** The first upcoming events the viewer sees. */
export function upcomingEventsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/events", { params: { query: { limit: aroundLimit } } });
}

const firstOf = <T>(page: { items: T[]; next_cursor?: string } | undefined): FirstOf<T> | undefined =>
  page === undefined
    ? undefined
    : {
        items: page.items.slice(0, aroundLimit),
        more: page.items.length > aroundLimit || page.next_cursor !== undefined,
      };

/**
 * The first open voice rooms the viewer sees; undefined until the server answered or when it failed, and nothing is
 * fetched while `enabled` is false (the voice rooms off, a reader without a session).
 */
export function useOpenVoiceRooms(enabled: boolean): FirstOf<VoiceRoom> | undefined {
  const query = useQuery({ ...openVoiceRoomsQuery(useAppData()), enabled });
  return enabled ? firstOf(query.data) : undefined;
}

/**
 * The first upcoming events the viewer sees; undefined until the server answered or when it failed, and nothing is
 * fetched while `enabled` is false (a reader without a session).
 */
export function useUpcomingEvents(enabled: boolean): FirstOf<ServerEvent> | undefined {
  const query = useQuery({ ...upcomingEventsQuery(useAppData()), enabled });
  return enabled ? firstOf(query.data) : undefined;
}
