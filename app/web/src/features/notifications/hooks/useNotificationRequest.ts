// SPDX-License-Identifier: AGPL-3.0-only
// One notification request and the notifications it holds (ADR-0158 §7, web-client/screens/notifications-request.md),
// and the answers to it: once answered, the page goes back to the list of requests and says so in a toast, since the
// page it was on is gone (G24). A failure stays on the page and says why under the answers (E2).
import { useToast } from "@my/ui";
import { useInfiniteQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useLoadFailure } from "../../../lib/loadFailure";
import { shellQueryRetry } from "../../../lib/query";
import {
  type NotificationRequest,
  notificationRequestNotificationsQueryOptions,
  notificationRequestQueryOptions,
  notificationRequestsPath,
  removeAnsweredRequest,
  type RequestAnswer,
} from "../api/requests";
import { groupNotifications, type NotificationGroup } from "./groupNotifications";
import { useAnswerRequest } from "./useNotificationRequests";

/** The notifications held in the request, as the page's feed shows them. */
export type HeldNotifications =
  | { status: "loading" }
  /** The first page failed; `message` is localized. */
  | { status: "error"; message: string; retry: () => void; isRetrying: boolean }
  | {
      status: "loaded";
      items: readonly NotificationGroup[];
      hasMore: boolean;
      isLoadingMore: boolean;
      loadMore: () => void;
    };

export type NotificationRequestView = {
  request: NotificationRequest;
  notifications: HeldNotifications;
  answer: (answer: RequestAnswer) => void;
  /** The answer being sent, or null. */
  pending: RequestAnswer | null;
  /** Why the last answer failed, localized; undefined while it did not. */
  error: string | undefined;
};

/** The request of account `accountId`, which the route's loader has loaded (a 404 is the route's error). */
export function useNotificationRequest(accountId: string): NotificationRequestView {
  const api = useApi();
  const queryClient = useQueryClient();
  const toast = useToast();
  const navigate = useNavigate();
  const request = useSuspenseQuery(notificationRequestQueryOptions(api, accountId)).data;
  // The route's loader has just tried the first page; a failure stays until the retry button.
  const list = useInfiniteQuery({
    ...notificationRequestNotificationsQueryOptions(api, accountId),
    retryOnMount: false,
    retry: shellQueryRetry,
  });
  const failure = useLoadFailure(list);
  const { mutation, doneMessage } = useAnswerRequest();

  const notifications = ((): HeldNotifications => {
    if (failure.error !== null && list.data === undefined) {
      return {
        status: "error",
        message: errorMessage(failure.error),
        retry: () => {
          void list.refetch();
        },
        isRetrying: failure.isRetrying,
      };
    }
    if (list.data === undefined) return { status: "loading" };
    return {
      status: "loaded",
      items: groupNotifications(list.data.pages.flatMap((page) => page.items)),
      hasMore: list.hasNextPage,
      isLoadingMore: list.isFetchingNextPage,
      loadMore: () => {
        if (!list.isFetchingNextPage) void list.fetchNextPage();
      },
    };
  })();

  return {
    request,
    notifications,
    answer: (answer) => {
      mutation.mutate(
        { request, answer },
        {
          onSuccess: () => {
            // The request's queries go once the page has been left, so that it does not read the request again.
            void navigate({ to: notificationRequestsPath }).then(() => {
              removeAnsweredRequest(api, queryClient, request.id);
              // The toast is announced too.
              toast.show({ message: doneMessage(request, answer), tone: "success" });
            });
          },
        },
      );
    },
    pending: mutation.isPending ? mutation.variables.answer : null,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
  };
}
