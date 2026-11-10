// SPDX-License-Identifier: AGPL-3.0-only
// The approval requests of the viewer's posts and the answers to them (ADR-0155 §10). An answered request leaves the
// cached pages at once, and the caller moves the focus to the next row; what the answer changed elsewhere (the post's
// counts, the thread an approved reply joins) goes into the cache first (cacheAnsweredRequest).
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { cacheAnsweredRequest, type InteractionRequest } from "../../status";
import {
  answerInteractionRequest,
  interactionRequestsQueryOptions,
  removeAnsweredRequest,
  type RequestAnswer,
} from "../api/interactionRequests";

export type InteractionRequests = {
  items: InteractionRequest[];
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => void;
  /** Answers `request`; `onDone` is called once it is off the list. A failure keeps the row and says why. */
  answer: (request: InteractionRequest, answer: RequestAnswer, onDone: () => void) => void;
  /** The request being answered, and how. */
  pending: { id: string; answer: RequestAnswer } | null;
};

/** What is announced once a request is answered, by its kind and the answer. */
const answered = {
  accept: {
    reply: "client:interaction_requests.accepted.reply",
    announce: "client:interaction_requests.accepted.announce",
    quote: "client:interaction_requests.accepted.quote",
  },
  reject: {
    reply: "client:interaction_requests.rejected.reply",
    announce: "client:interaction_requests.rejected.announce",
    quote: "client:interaction_requests.rejected.quote",
  },
} as const satisfies Record<RequestAnswer, Record<InteractionRequest["kind"], ClientMessageKey>>;

/** The requests that wait for the viewer's approval, newest first, and answering them. */
export function useInteractionRequests(): InteractionRequests {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const query = useSuspenseInfiniteQuery(interactionRequestsQueryOptions(api));
  const mutation = useMutation({
    mutationFn: ({ id, answer }: { id: string; answer: RequestAnswer }) => answerInteractionRequest(api, id, answer),
    // The request stays pending until the post's counts and the threads are in the cache (S1).
    onSuccess: (request) => cacheAnsweredRequest(queryClient, request),
  });
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
        { id: request.id, answer },
        {
          onSuccess: () => {
            removeAnsweredRequest(api, queryClient, request.id);
            onDone();
            notice.inPlace(
              t(answered[answer][request.kind], { name: request.account.display_name || request.account.username }),
            );
          },
          // A failure keeps the row and its focus, and says why (web-client/README.md E2).
          onError: (error) => {
            notice.inPlace(errorMessage(error), "assertive");
          },
        },
      );
    },
    pending: mutation.isPending ? mutation.variables : null,
  };
}
