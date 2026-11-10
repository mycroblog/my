// SPDX-License-Identifier: AGPL-3.0-only
// The notification requests and the answers to them (ADR-0158 §7, web-client/screens/notifications-requests.md). An
// answered request leaves the cached pages once what it changed elsewhere (the counts, and for an accepted one the
// notifications and the unread count) has been read again, as the caller moves the focus to the next row.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  answerNotificationRequest,
  cacheAnsweredRequest,
  type NotificationRequest,
  notificationRequestsQueryOptions,
  removeAnsweredRequest,
  type RequestAnswer,
} from "../api/requests";

export type NotificationRequestList = {
  items: NotificationRequest[];
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => void;
  /** Answers `request`; `onDone` is called once it is off the list. A failure keeps the row and says why. */
  answer: (request: NotificationRequest, answer: RequestAnswer, onDone: () => void) => void;
  /** The request being answered, and how. */
  pending: { id: string; answer: RequestAnswer } | null;
};

/** The display name of the source of a request, or its username. */
export function requestName(request: NotificationRequest): string {
  return request.account.display_name || request.account.username;
}

/** Answering a request: the mutation, and what is said once it is done. */
export function useAnswerRequest() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: ({ request, answer }: { request: NotificationRequest; answer: RequestAnswer }) =>
      answerNotificationRequest(api, request.id, answer),
    // The request stays pending until the queries on screen have been read again (S1).
    onSuccess: (_, { answer }) => cacheAnsweredRequest(api, queryClient, answer),
  });
  const doneMessage = (request: NotificationRequest, answer: RequestAnswer) =>
    t(answer === "accept" ? "client:notification_requests.accepted" : "client:notification_requests.dismissed", {
      name: requestName(request),
    });
  return { mutation, doneMessage };
}

/** The requests, the one with the newest notification first, which the route's loader has loaded, and answering them. */
export function useNotificationRequests(): NotificationRequestList {
  const api = useApi();
  const notice = useNotice();
  const query = useSuspenseInfiniteQuery(notificationRequestsQueryOptions(api));
  const queryClient = useQueryClient();
  const { mutation, doneMessage } = useAnswerRequest();
  return {
    items: query.data.pages.flatMap((page) => page.items),
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => {
      // A refetch in progress would overwrite the page fetched now (TanStack Query, "Infinite Queries").
      if (!query.isFetching) void query.fetchNextPage();
    },
    answer: (request, answer, onDone) => {
      mutation.mutate(
        { request, answer },
        {
          onSuccess: () => {
            removeAnsweredRequest(api, queryClient, request.id);
            onDone();
            notice.inPlace(doneMessage(request, answer));
          },
          // A failure keeps the row and its focus, and says why (web-client/README.md E2).
          onError: (error) => {
            notice.inPlace(errorMessage(error), "assertive");
          },
        },
      );
    },
    pending: mutation.isPending ? { id: mutation.variables.request.id, answer: mutation.variables.answer } : null,
  };
}
