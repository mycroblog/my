// SPDX-License-Identifier: AGPL-3.0-only
// What the page of an event (`/events/{id}`, ADR-0161 §8, §9, §15) shows and does. A signed-in user who sees the event
// gets it as the server shows it to that user (the places the user does not see never reach the client, only their
// kinds), with the answer, the attendees, the `.ics` file, and for its organizers the approvals, the invitations,
// the co-organizers (the organizer only), and the cancellation. An event with a voice chat place is asked for again at
// its start, so that the way into its room shows once the room opens. A visitor who is not signed in learns that only the
// local users take part, with the way to sign in; an event the user does not see is not found, as an unknown one.
import { useNotice } from "@my/ui";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useLoadFailure } from "../../../lib/loadFailure";
import { shellQueryRetry } from "../../../lib/query";
import {
  answerEventMutation,
  approvalMutation,
  cacheApproval,
  cacheEvent,
  cancelEventMutation,
  type Event,
  type EventAnswer,
  type EventAttendee,
  eventAttendeesQueryOptions,
  eventQueryOptions,
  fetchEventCalendar,
} from "../api/events";
import { saveFile } from "../../../lib/saveFile";
import { accountName, isolate } from "./names";
import { useEventFailureMessage } from "./useEventFailureMessage";

/** What the user may do with the event, by who the user is to it. */
export type EventRole = "organizer" | "coorganizer" | "guest";

export type EventPage =
  | { status: "loading" }
  | { status: "signed-out" }
  | { status: "not-found" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | { status: "ready"; event: Event; role: EventRole };

function isNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

/** The role of the user in `event`. */
function roleOf(event: Event): EventRole {
  if (event.viewer.organizer) return "organizer";
  if (event.viewer.coorganizer) return "coorganizer";
  return "guest";
}

/** How often the page asks again whether the room of the voice place opened, from the start on. */
const roomPollInterval = 10_000;
/** How long after the start the page waits for the room before it stops asking (the room waits 30 minutes). */
const roomPollWindow = 30 * 60_000;
/** The longest wait before the start that the page sets at once; it asks again then and waits the rest. */
const roomWaitCap = 60 * 60_000;

/**
 * When the page asks for the event again so that the entry to its voice chat room shows at the start (ADR-0161 §10):
 * the start itself, then every few seconds until the room opened or could not, for as long as the room waits. Never
 * for an event without a voice place, one with no date, or one cancelled.
 */
export function roomRefetchDelay(event: Event, now: number): number | false {
  const voice = event.places.find((place) => place.kind === "voice");
  if (voice === undefined || event.state !== "scheduled") return false;
  if (voice.room_id !== undefined || voice.room_unavailable !== undefined) return false;
  const start = Date.parse(event.starts_at);
  if (now < start) return Math.min(start - now + 1_000, roomWaitCap);
  return now - start < roomPollWindow ? roomPollInterval : false;
}

export function useEventPage(id: string): EventPage {
  const api = useApi();
  const session = useSessionStatus();
  const signedIn = session === "signed-in";
  // The loader has asked already: a refusal it got (a hidden or unknown event) is not asked again on mount.
  const query = useQuery({
    ...eventQueryOptions(api, id),
    enabled: signedIn,
    retryOnMount: false,
    refetchInterval: (q) => (q.state.data === undefined ? false : roomRefetchDelay(q.state.data, Date.now())),
  });
  const failure = useLoadFailure(query);
  if (session === "unknown") return { status: "loading" };
  if (session === "signed-out") return { status: "signed-out" };
  if (isNotFound(query.error)) return { status: "not-found" };
  if (query.data !== undefined) return { status: "ready", event: query.data, role: roleOf(query.data) };
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

/** The user's answer to an event: the current one (or the one being sent), and changing it. */
export type EventAnswerControl = {
  /** The answer shown: the one being sent while it is, else the server's; undefined before the first. */
  answer: EventAnswer | undefined;
  /** Whether the event takes answers now: it has a date, has not started, and was not cancelled. */
  open: boolean;
  change: (answer: EventAnswer) => void;
  isPending: boolean;
  /** Why the last answer was refused, localized. */
  error: string | undefined;
};

export function useEventAnswer(event: Event, started: boolean): EventAnswerControl {
  const api = useApi();
  const queryClient = useQueryClient();
  const failed = useEventFailureMessage();
  const answer = useMutation({
    ...answerEventMutation(api, event.id),
    onSuccess: (saved) => cacheEvent(api, queryClient, saved),
  });
  return {
    // The answer pressed shows at once (web-client/README.md S8), and goes back if the server refuses it.
    answer: answer.isPending ? answer.variables : event.viewer.answer,
    open: event.state === "scheduled" && !started,
    change: (value) => {
      if (answer.isPending || value === event.viewer.answer) return;
      answer.mutate(value);
    },
    isPending: answer.isPending,
    error: answer.isError ? failed(answer.error) : undefined,
  };
}

/** The attendees of an event (going and maybe), and, for its organizers, approving them. */
export type EventAttendeeList =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      attendees: readonly EventAttendee[];
      hasMore: boolean;
      loadMore: () => void;
      isLoadingMore: boolean;
      /** Approves an attendee, or revokes the approval; organizers only. */
      setApproval: (attendee: EventAttendee, approve: boolean) => void;
      /** The account whose approval is being changed. */
      pendingId: string | undefined;
      /** Why the last approval failed, localized. */
      approvalError: string | undefined;
    };

export function useEventAttendees(event: Event): EventAttendeeList {
  const api = useApi();
  const t = useT();
  const notice = useNotice();
  const queryClient = useQueryClient();
  const failed = useEventFailureMessage();
  const query = useInfiniteQuery({
    ...eventAttendeesQueryOptions(api, event.id),
    retry: shellQueryRetry,
    retryOnMount: false,
  });
  const failure = useLoadFailure(query);
  const approval = useMutation({
    ...approvalMutation(api, event.id),
    onSuccess: () => cacheApproval(api, queryClient, event.id),
  });
  if (query.data !== undefined) {
    return {
      status: "ready",
      attendees: query.data.pages.flatMap((page) => page.items),
      hasMore: query.hasNextPage,
      loadMore: () => {
        if (!query.isFetching) void query.fetchNextPage();
      },
      isLoadingMore: query.isFetchingNextPage,
      setApproval: (attendee, approve) => {
        if (approval.isPending) return;
        const name = accountName(attendee.account);
        approval.mutate(
          { accountId: attendee.account.id, approve },
          {
            onSuccess: () => {
              notice.inPlace(
                t(approve ? "client:events.attendees.approved_notice" : "client:events.attendees.revoked_notice", {
                  name: isolate(name),
                }),
              );
            },
          },
        );
      },
      pendingId: approval.isPending ? approval.variables.accountId : undefined,
      approvalError: approval.isError ? failed(approval.error) : undefined,
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

/** Saving the event's `.ics` file, to add it to the user's own calendar. */
export function useEventCalendar(event: Event) {
  const api = useApi();
  const failed = useEventFailureMessage();
  const download = useMutation({
    mutationKey: ["get", "/events/{id}/calendar.ics", event.id] as const,
    mutationFn: () => fetchEventCalendar(api, event.id),
    onSuccess: (file) => {
      saveFile(file, `${event.id}.ics`);
    },
  });
  return {
    save: () => {
      if (!download.isPending) download.mutate();
    },
    isPending: download.isPending,
    error: download.isError ? failed(download.error) : undefined,
  };
}

/** Cancelling the event, by its organizer, with an optional reason that the event shows. */
export function useCancelEvent(event: Event) {
  const api = useApi();
  const t = useT();
  const notice = useNotice();
  const queryClient = useQueryClient();
  const failed = useEventFailureMessage();
  const cancel = useMutation({
    ...cancelEventMutation(api, event.id),
    onSuccess: (saved) => cacheEvent(api, queryClient, saved),
  });
  return {
    run: (note: string, close: () => void) => {
      if (cancel.isPending) return;
      cancel.mutate(note.trim(), {
        onSuccess: () => {
          close();
          notice.inPlace(t("client:events.cancel.done"));
        },
      });
    },
    reset: () => {
      cancel.reset();
    },
    isPending: cancel.isPending,
    error: cancel.isError ? failed(cancel.error) : undefined,
  };
}
