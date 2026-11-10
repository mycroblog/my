// SPDX-License-Identifier: AGPL-3.0-only
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useLoadFailure } from "../../../lib/loadFailure";
import { shellQueryRetry } from "../../../lib/query";
import {
  type Conversation,
  cacheConversation,
  conversationsQueryOptions,
  markConversationRead,
} from "../api/conversations";

/** The direct messages page's list. */
export type DirectMessageList =
  | { status: "signed-out" }
  /** The session is resuming, or the first page is loading. */
  | { status: "loading" }
  /** The first page failed; `message` is localized. `isRetrying` while `retry` loads it again. */
  | { status: "error"; message: string; retry: () => void; isRetrying: boolean }
  | {
      status: "loaded";
      /** The loaded conversations, the one with the newest direct post first. */
      items: readonly Conversation[];
      hasMore: boolean;
      isLoadingMore: boolean;
      loadMore: () => void;
      /** The user opens a conversation's thread: an unread one is marked read on every device. */
      open: (conversation: Conversation) => void;
    };

/**
 * Marks a conversation read when its thread is opened from the list: what the user did, so an event handler sends it
 * (web-client/README.md R7). The answer replaces the conversation in the list's cache (S1), so the navigation's count
 * drops at once; the callbacks are the mutation's own, which still run after the list's page is left. A failure is
 * left alone: the conversation stays unread, and opening it again sends it again.
 */
function useMarkRead(): (conversation: Conversation) => void {
  const api = useApi();
  const queryClient = useQueryClient();
  const { mutate } = useMutation({
    mutationFn: (id: string) => markConversationRead(api, id),
    onSuccess: (conversation) => {
      cacheConversation(queryClient, api, conversation);
    },
  });
  return (conversation) => {
    if (conversation.unread) mutate(conversation.id);
  };
}

/** The user's conversations of the direct messages, loaded a page at a time. */
export function useDirectMessages(): DirectMessageList {
  const api = useApi();
  const session = useSessionStatus();
  // The route's loader has just tried the first page; a failure stays until the retry button or the return
  // online (lib/query.ts) instead of being tried again as the list mounts.
  const list = useInfiniteQuery({
    ...conversationsQueryOptions(api),
    enabled: session === "signed-in",
    retryOnMount: false,
    retry: shellQueryRetry,
  });
  const failure = useLoadFailure(list);
  const open = useMarkRead();

  if (session === "signed-out") return { status: "signed-out" };
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
    items: list.data.pages.flatMap((page) => page.items),
    hasMore: list.hasNextPage,
    isLoadingMore: list.isFetchingNextPage,
    loadMore: () => {
      if (!list.isFetchingNextPage) void list.fetchNextPage();
    },
    open,
  };
}
