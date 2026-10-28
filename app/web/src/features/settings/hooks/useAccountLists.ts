// SPDX-License-Identifier: AGPL-3.0-only
// The lists of blocked and muted accounts and of follow requests, and their actions (FR-302, FR-303). An
// account that an action takes off a list leaves the cached pages at once, and the caller moves focus to the
// next row, so that it is not lost with the removed row.
import { useAnnouncer } from "@my/ui";
import { type InfiniteData, useMutation, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useViewer } from "../../session";
import {
  type Account,
  accountActionMutation,
  blocksQueryOptions,
  cacheListChange,
  followRequestsQueryOptions,
  type ListChange,
  type MutedAccount,
  mutesQueryOptions,
  type Relationship,
  removeFromList,
} from "../api/relationships";

/** What a list screen needs from its list: the items so far, and loading more. */
export type AccountList<T> = {
  items: T[];
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => void;
};

type InfiniteList<P> = {
  data: InfiniteData<P>;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  isFetching: boolean;
  fetchNextPage: () => Promise<unknown>;
};

function toList<P, T>(query: InfiniteList<P>, itemsOf: (page: P) => T[]): AccountList<T> {
  return {
    items: query.data.pages.flatMap(itemsOf),
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => {
      // A refetch in progress would overwrite the page fetched now (TanStack Query, "Infinite Queries").
      if (!query.isFetching) void query.fetchNextPage();
    },
  };
}

/** An action on one account of a list, which takes it off the list when it succeeds. */
export type AccountAction = {
  /** Runs the action for `account`; `onDone` is called once the account is off the list. */
  run: (account: Account, onDone: () => void) => void;
  /** The account the action is running for. */
  pendingId: string | undefined;
};

type MutationCallbacks = { onSuccess: () => void; onError: (error: unknown) => void };

/**
 * The `onSuccess` of a mutation that makes `change` to the viewer's relationship with the account it is given:
 * the action is done once what the change shows elsewhere is in the cache (cacheListChange; the row stays
 * pending until then), so that a page opened next shows it at once (web-client/README.md S1). The list itself is
 * updated by the action (useListAction), which then moves focus.
 */
function useCacheListChange(change: ListChange): (relationship: Relationship) => Promise<void> {
  const queryClient = useQueryClient();
  const viewer = useViewer();
  const viewerId = viewer.status === "signed-in" ? viewer.account.id : "";
  return (relationship) => cacheListChange(queryClient, change, relationship, viewerId);
}

/** An AccountAction from a mutation of one account, the change it makes, and its announcement. */
function useListAction(options: {
  mutate: (id: string, callbacks: MutationCallbacks) => void;
  pendingId: string | undefined;
  change: ListChange;
  done: (name: string) => string;
}): AccountAction {
  const api = useApi();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  return {
    pendingId: options.pendingId,
    run: (account, onDone) => {
      options.mutate(account.id, {
        onSuccess: () => {
          removeFromList(queryClient, api, options.change, account.id);
          onDone();
          announce(options.done(account.display_name || account.username));
        },
        // A failure keeps the row and its focus, and says why (web-client/README.md E2).
        onError: (error) => {
          announce(errorMessage(error), "assertive");
        },
      });
    },
  };
}

/** The blocked accounts and unblocking them. */
export function useBlocks(): { list: AccountList<Account>; unblock: AccountAction } {
  const api = useApi();
  const t = useT();
  const query = useSuspenseInfiniteQuery(blocksQueryOptions(api));
  const unblock = useMutation({
    ...accountActionMutation(api, "/accounts/{id}/unblock"),
    onSuccess: useCacheListChange("unblock"),
  });
  return {
    list: toList(query, (page) => page.accounts),
    unblock: useListAction({
      mutate: (id, callbacks) => {
        unblock.mutate(id, callbacks);
      },
      pendingId: unblock.isPending ? unblock.variables : undefined,
      change: "unblock",
      done: (name) => t("client:settings.blocks.unblocked", { name }),
    }),
  };
}

/** The muted accounts and unmuting them. */
export function useMutes(): { list: AccountList<MutedAccount>; unmute: AccountAction } {
  const api = useApi();
  const t = useT();
  const query = useSuspenseInfiniteQuery(mutesQueryOptions(api));
  const unmute = useMutation({
    ...accountActionMutation(api, "/accounts/{id}/unmute"),
    onSuccess: useCacheListChange("unmute"),
  });
  return {
    list: toList(query, (page) => page.mutes),
    unmute: useListAction({
      mutate: (id, callbacks) => {
        unmute.mutate(id, callbacks);
      },
      pendingId: unmute.isPending ? unmute.variables : undefined,
      change: "unmute",
      done: (name) => t("client:settings.mutes.unmuted", { name }),
    }),
  };
}

/** The follow requests to the viewer, and accepting or rejecting them. */
export function useFollowRequests(): { list: AccountList<Account>; accept: AccountAction; reject: AccountAction } {
  const api = useApi();
  const t = useT();
  const query = useSuspenseInfiniteQuery(followRequestsQueryOptions(api));
  // The viewer has one more follower.
  const accept = useMutation({
    ...accountActionMutation(api, "/follow_requests/{id}/authorize"),
    onSuccess: useCacheListChange("accept"),
  });
  const reject = useMutation({
    ...accountActionMutation(api, "/follow_requests/{id}/reject"),
    onSuccess: useCacheListChange("reject"),
  });
  return {
    list: toList(query, (page) => page.accounts),
    accept: useListAction({
      mutate: (id, callbacks) => {
        accept.mutate(id, callbacks);
      },
      pendingId: accept.isPending ? accept.variables : undefined,
      change: "accept",
      done: (name) => t("client:settings.follow_requests.accepted", { name }),
    }),
    reject: useListAction({
      mutate: (id, callbacks) => {
        reject.mutate(id, callbacks);
      },
      pendingId: reject.isPending ? reject.variables : undefined,
      change: "reject",
      done: (name) => t("client:settings.follow_requests.rejected", { name }),
    }),
  };
}
