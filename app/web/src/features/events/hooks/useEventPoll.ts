// SPDX-License-Identifier: AGPL-3.0-only
// The date poll of an event that has no date yet (ADR-0161 §7): its candidate dates with how many answered each way
// and the user's own answer, saved at once; who answered a candidate; and, for the organizers, adding a candidate
// (typed as the wall clock of the event's time zone, as the editor's dates are), withdrawing one, and confirming one,
// which fixes the event's date and ends the poll.
import { useNotice } from "@my/ui";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useNow } from "../../../lib/clock";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useLoadFailure } from "../../../lib/loadFailure";
import {
  addCandidateMutation,
  cacheCandidates,
  cacheEvent,
  confirmCandidateMutation,
  type Event,
  type EventBallot,
  eventBallotsQueryOptions,
  type EventCandidate,
  eventCandidatesQueryOptions,
  eventLimits,
  type EventVote,
  voteMutation,
  withdrawCandidateMutation,
} from "../api/events";
import { fromZonedInput } from "./eventTime";
import { useEventFailureMessage } from "./useEventFailureMessage";

/** The candidate dates of the poll as the page shows them. */
export type EventPoll =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      candidates: readonly EventCandidate[];
      /** The user's answer shown for `candidate`: the one being sent while it is, else the server's. */
      voteOf: (candidate: EventCandidate) => EventVote | undefined;
      vote: (candidate: EventCandidate, vote: EventVote) => void;
      /** The candidate whose answer is being sent. */
      votingId: string | undefined;
      /** Why the last answer was refused, localized. */
      voteError: string | undefined;
    };

/** The candidate dates of `event`'s poll, and answering them. */
export function useEventPoll(event: Event): EventPoll {
  const api = useApi();
  const queryClient = useQueryClient();
  const failed = useEventFailureMessage();
  const query = useQuery({ ...eventCandidatesQueryOptions(api, event.id), retryOnMount: false });
  const failure = useLoadFailure(query);
  const vote = useMutation({
    ...voteMutation(api, event.id),
    onSuccess: () => cacheCandidates(api, queryClient, event.id),
  });
  if (query.data !== undefined) {
    return {
      status: "ready",
      candidates: query.data.items,
      // The answer pressed shows at once (web-client/README.md S8), and goes back if the server refuses it.
      voteOf: (candidate) =>
        vote.isPending && vote.variables.candidateId === candidate.id ? vote.variables.vote : candidate.vote,
      vote: (candidate, value) => {
        if (vote.isPending || value === candidate.vote) return;
        vote.mutate({ candidateId: candidate.id, vote: value });
      },
      votingId: vote.isPending ? vote.variables.candidateId : undefined,
      voteError: vote.isError ? failed(vote.error) : undefined,
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

/** Who answered a candidate date, each with the answer, loaded when the dialog that lists them opens. */
export type EventBallots =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      ballots: readonly EventBallot[];
      hasMore: boolean;
      loadMore: () => void;
      isLoadingMore: boolean;
    };

export function useEventBallots(event: Event, candidate: EventCandidate): EventBallots {
  const api = useApi();
  const query = useInfiniteQuery(eventBallotsQueryOptions(api, event.id, candidate.id));
  const failure = useLoadFailure(query);
  if (query.data !== undefined) {
    return {
      status: "ready",
      ballots: query.data.pages.flatMap((page) => page.items),
      hasMore: query.hasNextPage,
      loadMore: () => {
        if (!query.isFetching) void query.fetchNextPage();
      },
      isLoadingMore: query.isFetchingNextPage,
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

/** A candidate's dates in the viewer's time zone, as the table's row header and the confirmations name it. */
export function useCandidateLabel(): (candidate: EventCandidate) => string {
  const intl = useIntl();
  return (candidate) =>
    intl.formatDateTimeRange(Date.parse(candidate.starts_at), Date.parse(candidate.ends_at), {
      weekday: "short",
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
}

/** The organizers' tools of the poll: confirming a candidate, withdrawing one, and adding one. */
export function useEventPollOrganizing(event: Event) {
  const api = useApi();
  const t = useT();
  const notice = useNotice();
  const queryClient = useQueryClient();
  const failed = useEventFailureMessage();
  const label = useCandidateLabel();
  const now = useNow();
  const confirm = useMutation({
    ...confirmCandidateMutation(api, event.id),
    // The notice is given here, before the event is replaced: the scheduled event drops the poll, and with it this
    // hook, whose callbacks of mutate would then never run.
    onSuccess: async (saved, candidate) => {
      notice.inPlace(t("client:events.poll.confirmed_notice", { date: label(candidate) }));
      await Promise.all([cacheEvent(api, queryClient, saved), cacheCandidates(api, queryClient, event.id)]);
    },
  });
  const withdraw = useMutation({
    ...withdrawCandidateMutation(api, event.id),
    onSuccess: () => cacheCandidates(api, queryClient, event.id),
  });
  const add = useMutation({
    ...addCandidateMutation(api, event.id),
    onSuccess: () => cacheCandidates(api, queryClient, event.id),
  });
  return {
    confirm: (candidate: EventCandidate, close: () => void) => {
      if (confirm.isPending) return;
      confirm.mutate(candidate, { onSuccess: close });
    },
    resetConfirm: () => {
      confirm.reset();
    },
    isConfirming: confirm.isPending,
    confirmError: confirm.isError ? failed(confirm.error) : undefined,
    withdraw: (candidate: EventCandidate) => {
      if (withdraw.isPending) return;
      withdraw.mutate(candidate, {
        onSuccess: () => {
          notice.inPlace(t("client:events.poll.withdrawn_notice", { date: label(candidate) }));
        },
      });
    },
    withdrawingId: withdraw.isPending ? withdraw.variables.id : undefined,
    withdrawError: withdraw.isError ? failed(withdraw.error) : undefined,
    /** Adds the candidate typed as the wall clock of the event's zone, and returns what its check refused (then nothing is sent). */
    add: (startsAt: string, endsAt: string, onAdded: () => void): CandidateFieldErrors => {
      const errors = checkCandidate(startsAt, endsAt, event.time_zone, now, t);
      if (errors.starts_at !== undefined || errors.ends_at !== undefined || add.isPending) return errors;
      const start = fromZonedInput(startsAt, event.time_zone);
      const end = fromZonedInput(endsAt, event.time_zone);
      if (start === undefined || end === undefined) return errors;
      add.mutate(
        { starts_at: start.toISOString(), ends_at: end.toISOString() },
        {
          onSuccess: (candidate) => {
            onAdded();
            notice.inPlace(t("client:events.poll.added_notice", { date: label(candidate) }));
          },
        },
      );
      return errors;
    },
    isAdding: add.isPending,
    addError: add.isError ? failed(add.error) : undefined,
  };
}

/** The messages of a candidate's start and end that a check refused. */
export type CandidateFieldErrors = { starts_at?: string; ends_at?: string };

/**
 * Checks a candidate as the server checks a date (ADR-0161 §5): a start after `now`, an end after the start and at
 * most 7 days later, both typed as the wall clock of `timeZone`.
 */
export function checkCandidate(
  startsAt: string,
  endsAt: string,
  timeZone: string,
  now: number,
  t: ReturnType<typeof useT>,
): CandidateFieldErrors {
  const errors: CandidateFieldErrors = {};
  const start = fromZonedInput(startsAt, timeZone);
  const end = fromZonedInput(endsAt, timeZone);
  if (start === undefined) errors.starts_at = t("client:events.form.starts_at.required");
  else if (start.getTime() <= now) errors.starts_at = t("client:events.form.starts_at.past");
  if (end === undefined) errors.ends_at = t("client:events.form.ends_at.required");
  else if (start !== undefined && end.getTime() <= start.getTime()) {
    errors.ends_at = t("client:events.form.ends_at.before_start");
  } else if (start !== undefined && end.getTime() - start.getTime() > eventLimits.duration) {
    errors.ends_at = t("client:events.form.ends_at.too_long");
  }
  return errors;
}
