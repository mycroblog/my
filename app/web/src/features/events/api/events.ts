// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the events of the local users (web-client/README.md S1, ADR-0161 §16): the upcoming
// events that the user sees (GET /events, also those an account organizes for its profile), a few of them for the
// third column, one event (GET /events/{id}, which leaves out the places that the user does not see), its attendees,
// its invitations (for the organizers), and the writes: creating and changing an event, cancelling it, answering it,
// approving an attendee, inviting, and naming co-organizers; and the date poll of an event that has no date yet: its
// candidate dates with their counts, who answered one, answering, and (for the organizers) adding, withdrawing, and
// confirming a candidate. Its `.ics` file (GET /events/{id}/calendar.ics) needs
// the user's token, so it is fetched and saved as a file instead of linked.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { shellQueryRetry } from "../../../lib/query";

type Schemas = components["schemas"];

export type Event = Schemas["Event"];
export type EventAccount = Schemas["Account"];
export type EventAnswer = Schemas["EventAnswer"];
export type EventAttendee = Schemas["EventAttendee"];
export type EventCreate = Schemas["EventCreate"];
export type EventInvitation = Schemas["EventInvitation"];
export type EventPlace = Schemas["EventPlace"];
export type EventPlaceDisclosure = Schemas["EventPlaceDisclosure"];
export type EventPlaceKind = Schemas["EventPlaceKind"];
export type EventUpdate = Schemas["EventUpdate"];
export type EventVisibility = Schemas["EventVisibility"];
export type EventCancelReason = Schemas["EventCancelReason"];
export type EventCandidate = Schemas["EventCandidate"];
export type EventCandidateCreate = Schemas["EventCandidateCreate"];
export type EventBallot = Schemas["EventBallot"];
export type EventVote = Schemas["EventVote"];
export type EventRoomUnavailableReason = Schemas["EventRoomUnavailableReason"];
export type VoiceRoomSpeakers = Schemas["VoiceRoomSpeakers"];

/** The visibilities of an event, from the widest, in the order of the form. */
export const eventVisibilities = [
  "public",
  "followers",
  "following",
  "mutuals",
  "invited",
] as const satisfies readonly EventVisibility[];

/** Who sees the physical and URL places, in the order of the form. */
export const eventPlaceDisclosures = ["approved", "viewers"] as const satisfies readonly EventPlaceDisclosure[];

/** The answers to an event, in the order of the buttons. */
export const eventAnswers = ["going", "maybe", "declined"] as const satisfies readonly EventAnswer[];

/** The answers to a candidate date, in the order of the table's columns and of the choices. */
export const eventVotes = ["yes", "maybe", "no"] as const satisfies readonly EventVote[];

/**
 * The limits of an event's settings, as the server checks them (internal/domain/gathering, ADR-0161 §5, §6): the
 * title in grapheme clusters (and in code points, `titleCodePoints`), the other texts in code points, the length of an
 * event in ms, and how far ahead it may start.
 */
export const eventLimits = {
  title: 100,
  titleCodePoints: 1000,
  description: 3000,
  placeName: 100,
  address: 500,
  url: 2000,
  capacity: 10_000,
  coorganizers: 3,
  cancelNote: 500,
  candidates: 20,
  duration: 7 * 24 * 60 * 60_000,
} as const;

/** How many events the third column and a profile show. */
const eventRailSize = 3;

/** How many events or people one page of a list holds. */
const pageSize = 20;

/** The page of an event, which its notifications link to. */
export function eventPath(id: string): string {
  return `/events/${id}`;
}

/** The editor of an event, for its organizers. */
export function eventEditPath(id: string): string {
  return `/events/${id}/edit`;
}

/** The upcoming events that the user sees (with `organizerId`, those the account organizes), nearest start first. */
export function eventsQueryOptions(api: Api, organizerId?: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/events", { organizer_id: organizerId }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = {
        limit: pageSize,
        ...(pageParam === undefined ? {} : { cursor: pageParam }),
        ...(organizerId === undefined ? {} : { organizer_id: organizerId }),
      };
      const { data, error } = await api.fetch.GET("/events", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The first upcoming events, for the third column, or those that `organizerId` organizes, for a profile. */
export function eventRailQueryOptions(api: Api, organizerId?: string) {
  return api.query.queryOptions("get", "/events", {
    params: { query: { limit: eventRailSize, ...(organizerId !== undefined && { organizer_id: organizerId }) } },
  });
}

/**
 * An event that the user sees; a 404 for one that the user does not see or that does not exist. A refusal (4xx) is not
 * tried again, so that the page says at once that the event is not there.
 */
export function eventQueryOptions(api: Api, id: string) {
  return {
    ...api.query.queryOptions("get", "/events/{id}", { params: { path: { id } } }),
    retry: shellQueryRetry,
  };
}

/** The attendees who answered going or maybe, in the order of their answers, a page at a time. */
export function eventAttendeesQueryOptions(api: Api, id: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/events/{id}/attendees", { id }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = pageParam === undefined ? { limit: pageSize } : { limit: pageSize, cursor: pageParam };
      const { data, error } = await api.fetch.GET("/events/{id}/attendees", {
        params: { path: { id }, query },
        signal,
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The invitations of an event that were not revoked, oldest first, for its organizers, a page at a time. */
export function eventInvitationsQueryOptions(api: Api, id: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/events/{id}/invitations", { id }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = pageParam === undefined ? { limit: pageSize } : { limit: pageSize, cursor: pageParam };
      const { data, error } = await api.fetch.GET("/events/{id}/invitations", {
        params: { path: { id }, query },
        signal,
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The candidate dates of an event's date poll, the earliest first, with their counts and the user's answers. */
export function eventCandidatesQueryOptions(api: Api, id: string) {
  return {
    ...api.query.queryOptions("get", "/events/{id}/candidates", { params: { path: { id } } }),
    retry: shellQueryRetry,
  };
}

/** Who answered a candidate date and how, a page at a time. */
export function eventBallotsQueryOptions(api: Api, id: string, candidateId: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/events/{id}/candidates/{candidate_id}/votes", { id, candidateId }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = pageParam === undefined ? { limit: pageSize } : { limit: pageSize, cursor: pageParam };
      const { data, error } = await api.fetch.GET("/events/{id}/candidates/{candidate_id}/votes", {
        params: { path: { id, candidate_id: candidateId }, query },
        signal,
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Answers a candidate date (PUT /events/{id}/candidates/{candidate_id}/vote). */
export function voteMutation(api: Api, id: string) {
  return {
    mutationKey: ["put", "/events/{id}/candidates/{candidate_id}/vote", id] as const,
    mutationFn: async ({ candidateId, vote }: { candidateId: string; vote: EventVote }): Promise<EventCandidate> => {
      const { data, error } = await api.fetch.PUT("/events/{id}/candidates/{candidate_id}/vote", {
        params: { path: { id, candidate_id: candidateId } },
        body: { vote },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Adds a candidate date to the poll (POST /events/{id}/candidates), by an organizer. */
export function addCandidateMutation(api: Api, id: string) {
  return {
    mutationKey: ["post", "/events/{id}/candidates", id] as const,
    mutationFn: async (body: EventCandidateCreate): Promise<EventCandidate> => {
      const { data, error } = await api.fetch.POST("/events/{id}/candidates", { params: { path: { id } }, body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Withdraws a candidate date of the poll (DELETE /events/{id}/candidates/{candidate_id}), by an organizer. */
export function withdrawCandidateMutation(api: Api, id: string) {
  return {
    mutationKey: ["delete", "/events/{id}/candidates/{candidate_id}", id] as const,
    mutationFn: async (candidate: EventCandidate): Promise<EventCandidate> => {
      const { error } = await api.fetch.DELETE("/events/{id}/candidates/{candidate_id}", {
        params: { path: { id, candidate_id: candidate.id } },
      });
      if (error !== undefined) throw new ApiError(error);
      return candidate;
    },
  };
}

/** Fixes the event's date to a candidate (POST /events/{id}/candidates/{candidate_id}/confirm), by an organizer. */
export function confirmCandidateMutation(api: Api, id: string) {
  return {
    mutationKey: ["post", "/events/{id}/candidates/{candidate_id}/confirm", id] as const,
    mutationFn: async (candidate: EventCandidate): Promise<Event> => {
      const { data, error } = await api.fetch.POST("/events/{id}/candidates/{candidate_id}/confirm", {
        params: { path: { id, candidate_id: candidate.id } },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** The local accounts whose names match `q`, for inviting and naming co-organizers; remote accounts are left out. */
export function localAccountSearchQueryOptions(api: Api, q: string) {
  return {
    ...api.query.queryOptions("get", "/accounts/search", { params: { query: { q, limit: 5 } } }),
    select: (data: { items: EventAccount[] }) => data.items.filter((account) => !account.acct.includes("@")),
    retry: false,
  };
}

/** The event's iCalendar file (GET /events/{id}/calendar.ics), with only the places that the user sees. */
export async function fetchEventCalendar(api: Api, id: string): Promise<Blob> {
  const { data, error } = await api.fetch.GET("/events/{id}/calendar.ics", {
    params: { path: { id } },
    parseAs: "blob",
  });
  if (data === undefined) throw new ApiError(error);
  return data;
}

/** Creates an event that the user organizes (POST /events), as the options of useMutation. */
export function createEventMutation(api: Api) {
  return {
    mutationKey: ["post", "/events"] as const,
    mutationFn: async (body: EventCreate): Promise<Event> => {
      const { data, error } = await api.fetch.POST("/events", { body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Changes an event (PATCH /events/{id}), by an organizer, as the options of useMutation. */
export function updateEventMutation(api: Api, id: string) {
  return {
    mutationKey: ["patch", "/events/{id}", id] as const,
    mutationFn: async (body: EventUpdate): Promise<Event> => {
      const { data, error } = await api.fetch.PATCH("/events/{id}", { params: { path: { id } }, body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Cancels an event (POST /events/{id}/cancel), by its organizer, with the reason when given. */
export function cancelEventMutation(api: Api, id: string) {
  return {
    mutationKey: ["post", "/events/{id}/cancel", id] as const,
    mutationFn: async (note: string): Promise<Event> => {
      const { data, error } = await api.fetch.POST("/events/{id}/cancel", {
        params: { path: { id } },
        body: note === "" ? {} : { note },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Answers an event (PUT /events/{id}/rsvp). */
export function answerEventMutation(api: Api, id: string) {
  return {
    mutationKey: ["put", "/events/{id}/rsvp", id] as const,
    mutationFn: async (answer: EventAnswer): Promise<Event> => {
      const { data, error } = await api.fetch.PUT("/events/{id}/rsvp", {
        params: { path: { id } },
        body: { answer },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Approves an attendee, or revokes the approval (PUT / DELETE /events/{id}/attendees/{account_id}/approval). */
export function approvalMutation(api: Api, id: string) {
  return {
    mutationKey: ["put", "/events/{id}/attendees/{account_id}/approval", id] as const,
    mutationFn: async ({ accountId, approve }: { accountId: string; approve: boolean }): Promise<EventAttendee> => {
      const params = { path: { id, account_id: accountId } };
      const { data, error } = approve
        ? await api.fetch.PUT("/events/{id}/attendees/{account_id}/approval", { params })
        : await api.fetch.DELETE("/events/{id}/attendees/{account_id}/approval", { params });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Invites a local user to an event (POST /events/{id}/invitations). */
export function inviteMutation(api: Api, id: string) {
  return {
    mutationKey: ["post", "/events/{id}/invitations", id] as const,
    mutationFn: async (account: EventAccount): Promise<EventInvitation> => {
      const { data, error } = await api.fetch.POST("/events/{id}/invitations", {
        params: { path: { id } },
        body: { account_id: account.id },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Revokes the invitation of a user (DELETE /events/{id}/invitations/{account_id}). */
export function revokeInvitationMutation(api: Api, id: string) {
  return {
    mutationKey: ["delete", "/events/{id}/invitations/{account_id}", id] as const,
    mutationFn: async (account: EventAccount): Promise<EventAccount> => {
      const { error } = await api.fetch.DELETE("/events/{id}/invitations/{account_id}", {
        params: { path: { id, account_id: account.id } },
      });
      if (error !== undefined) throw new ApiError(error);
      return account;
    },
  };
}

/** Names a co-organizer, or removes one (PUT / DELETE /events/{id}/coorganizers/{account_id}), by the organizer. */
export function coorganizerMutation(api: Api, id: string) {
  return {
    mutationKey: ["put", "/events/{id}/coorganizers/{account_id}", id] as const,
    mutationFn: async ({ account, assign }: { account: EventAccount; assign: boolean }): Promise<Event> => {
      const params = { path: { id, account_id: account.id } };
      const { data, error } = assign
        ? await api.fetch.PUT("/events/{id}/coorganizers/{account_id}", { params })
        : await api.fetch.DELETE("/events/{id}/coorganizers/{account_id}", { params });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Marks stale every list of events (the list page, the third column, the profiles): an event may enter, leave, or move. */
function refreshEventLists(queryClient: QueryClient): Promise<void> {
  return queryClient.invalidateQueries({
    predicate: (query) => query.queryKey[0] === "get" && query.queryKey[1] === "/events",
  });
}

/**
 * Puts an event as the server answered a write into its query (ADR-0028), and marks stale the lists, where it may
 * move, and its attendees, whose answers and approvals the write may change.
 */
export async function cacheEvent(api: Api, queryClient: QueryClient, event: Event): Promise<void> {
  queryClient.setQueryData(eventQueryOptions(api, event.id).queryKey, event);
  await Promise.all([
    refreshEventLists(queryClient),
    queryClient.invalidateQueries({ queryKey: eventAttendeesQueryOptions(api, event.id).queryKey }),
  ]);
}

/** Marks stale the event and its attendees after an approval changed: the attendee's mark moves. */
export async function cacheApproval(api: Api, queryClient: QueryClient, id: string): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: eventAttendeesQueryOptions(api, id).queryKey });
}

/**
 * Marks stale the candidates of an event and who answered them, after an answer, a new candidate, or a withdrawal,
 * and the event too, whose earliest start and latest end follow its candidates.
 */
export async function cacheCandidates(api: Api, queryClient: QueryClient, id: string): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: eventCandidatesQueryOptions(api, id).queryKey }),
    queryClient.invalidateQueries({
      predicate: (query) =>
        query.queryKey[1] === "/events/{id}/candidates/{candidate_id}/votes" &&
        (query.queryKey[2] as { id?: string } | undefined)?.id === id,
    }),
    queryClient.invalidateQueries({ queryKey: eventQueryOptions(api, id).queryKey }),
  ]);
}

/** Marks stale the invitations of an event after one was sent or revoked. */
export async function cacheInvitations(api: Api, queryClient: QueryClient, id: string): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: eventInvitationsQueryOptions(api, id).queryKey });
}

/** Why a write to an event failed, by the problem type of the server (ADR-0161 §16). */
export type EventFailure =
  | "not-found"
  | "full"
  | "closed"
  | "unscheduled"
  | "scheduled"
  | "last-candidate"
  | "not-eligible"
  | "voice-unavailable"
  | "already-invited"
  | "invalid-invitee"
  | "invalid-coorganizer"
  | "too-many-coorganizers"
  | "not-organizer"
  | "organizer-only"
  | "rate-limited"
  | "invalid"
  | "other";

export function eventFailure(error: unknown): EventFailure {
  if (!(error instanceof ApiError)) return "other";
  if (error.status === 404) return "not-found";
  if (error.status === 429) return "rate-limited";
  switch (error.type) {
    case "/api/v1/problems/event-full":
      return "full";
    case "/api/v1/problems/event-closed":
      return "closed";
    case "/api/v1/problems/event-unscheduled":
      return "unscheduled";
    case "/api/v1/problems/event-scheduled":
      return "scheduled";
    case "/api/v1/problems/event-last-candidate":
      return "last-candidate";
    case "/api/v1/problems/event-not-eligible":
      return "not-eligible";
    case "/api/v1/problems/event-voice-unavailable":
      return "voice-unavailable";
    case "/api/v1/problems/event-already-invited":
      return "already-invited";
    case "/api/v1/problems/event-invalid-invitee":
      return "invalid-invitee";
    case "/api/v1/problems/event-invalid-coorganizer":
      return "invalid-coorganizer";
    case "/api/v1/problems/event-too-many-coorganizers":
      return "too-many-coorganizers";
    case "/api/v1/problems/event-not-organizer":
      return "not-organizer";
    case "/api/v1/problems/event-organizer-only":
      return "organizer-only";
    default:
      return error.status === 422 ? "invalid" : "other";
  }
}
