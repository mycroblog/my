// SPDX-License-Identifier: AGPL-3.0-only
// The members of one list, in the region that "Manage members" opens under its row (web-client/screens/lists.md §3):
// the members a page at a time, the search of the accounts the user follows, and adding and removing a member at once.
// The hooks return no JSX.
import { useNotice } from "@my/ui";
import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  type Account,
  accountListsQueryOptions,
  addListMemberMutation,
  cacheMemberChange,
  followingSearchQueryOptions,
  type List,
  listFailure,
  listMembersQueryOptions,
  maxListMembers,
  removeListMemberMutation,
} from "../api/lists";
import { accountName, isolate, useLimits } from "./names";

/** The members as the region shows them. */
export type ListMembers =
  | { status: "loading" }
  | { status: "error"; message: string; retry: () => void; isRetrying: boolean }
  | {
      status: "done";
      members: readonly Account[];
      hasMore: boolean;
      loadMore: () => void;
      isLoadingMore: boolean;
      /** Why the last page failed to load, localized; undefined when it did not. */
      loadMoreError: string | undefined;
    };

export function useListMembers(list: List): ListMembers {
  const api = useApi();
  const query = useInfiniteQuery(listMembersQueryOptions(api, list.id));
  if (query.data === undefined) {
    if (query.isError)
      return {
        status: "error",
        message: errorMessage(query.error),
        retry: () => {
          void query.refetch();
        },
        isRetrying: query.isFetching,
      };
    return { status: "loading" };
  }
  return {
    status: "done",
    members: query.data.pages.flatMap((page) => page.accounts),
    hasMore: query.hasNextPage,
    loadMore: () => {
      if (query.hasNextPage && !query.isFetchingNextPage) void query.fetchNextPage();
    },
    isLoadingMore: query.isFetchingNextPage,
    loadMoreError: query.isFetchNextPageError ? errorMessage(query.error) : undefined,
  };
}

/** How long the typing pauses before the search is sent, in milliseconds. */
const searchDelay = 300;

/** The longest text the name search takes (api/openapi.yaml, searchAccounts `q` after its `@`). */
const maxSearchQuery = 100;

/** The followed accounts whose names match the typed text. */
export type FollowingSearch =
  /** Nothing to search: the field is empty, or the text is not a name. */
  | { status: "idle" }
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "done"; accounts: readonly Account[] };

/** The text as the name search takes it, or null when it is not one (empty, too long). */
function searchQueryOf(text: string): string | null {
  const q = text.trim().replace(/^@/u, "");
  if (q === "" || Array.from(q).length > maxSearchQuery) return null;
  return q;
}

/** Searches the accounts the user follows by the start of their names, once the typing pauses. */
export function useFollowingSearch(text: string): FollowingSearch {
  const api = useApi();
  const next = searchQueryOf(text);
  const [query, setQuery] = useState(next);
  useEffect(() => {
    // The search follows the typed text once it pauses (a timer, an external system: react.dev "Synchronizing with Effects").
    const timer = setTimeout(() => {
      setQuery(next);
    }, searchDelay);
    return () => {
      clearTimeout(timer);
    };
  }, [next]);
  const result = useQuery({
    ...followingSearchQueryOptions(api, query ?? ""),
    enabled: query !== null,
    // The results stay while the next text is searched, rather than flashing empty at every key.
    placeholderData: keepPreviousData,
  });
  if (next === null) return { status: "idle" };
  if (result.isError) return { status: "error", message: errorMessage(result.error) };
  if (result.data === undefined) return { status: "loading" };
  return { status: "done", accounts: result.data };
}

/**
 * Whether each of `accounts` (the search's results) is in the list, by the lists each account is in
 * (GET /accounts/{id}/lists, one per result; adding a member reads its lists again): undefined while it loads.
 */
export function useMembership(list: List, accounts: readonly Account[]): (account: Account) => boolean | undefined {
  const api = useApi();
  const results = useQueries({
    queries: accounts.map((account) => accountListsQueryOptions(api, account.id)),
  });
  return (account) => {
    const lists = results[accounts.indexOf(account)]?.data;
    return lists === undefined ? undefined : lists.items.some((item) => item.id === list.id);
  };
}

/** Adding and removing the members of one list. */
export type MemberChanges = {
  /** Whether the list has the most members, so that nobody more can be added. */
  isFull: boolean;
  /** Adds `account`; `onDone` is called once it is a member. */
  add: (account: Account, onDone: () => void) => void;
  /** The ID of the account being added. */
  addingId: string | undefined;
  /** Why adding the account failed, localized, shown beside its result; undefined for the others. */
  addError: (account: Account) => string | undefined;
  /** Removes `account`; `onDone` is called once it is off the list. */
  remove: (account: Account, onDone: () => void) => void;
  /** The ID of the account being removed. */
  removingId: string | undefined;
  /** Why the last removal failed, localized, shown above the members; undefined when it did not. */
  removeError: string | undefined;
};

export function useMemberChanges(list: List): MemberChanges {
  const api = useApi();
  const t = useT();
  const limits = useLimits();
  const queryClient = useQueryClient();
  const notice = useNotice();
  // The cache is updated before the mutation settles, so that the change shows without going back for a moment.
  const add = useMutation({
    ...addListMemberMutation(api),
    onSuccess: (change) => cacheMemberChange(api, queryClient, change, 1),
  });
  const remove = useMutation({
    ...removeListMemberMutation(api),
    onSuccess: (change) => cacheMemberChange(api, queryClient, change, -1),
  });
  const addFailure = (error: unknown) => {
    switch (listFailure(error)) {
      case "too-many-members":
        return t("client:lists.members.limit", { max: limits.members });
      case "not-followed":
        return t("client:lists.members.not_followed");
      default:
        return t("client:lists.members.add_failed", { reason: errorMessage(error) });
    }
  };
  return {
    isFull: list.members_count >= maxListMembers,
    add: (account, onDone) => {
      remove.reset();
      add.mutate(
        { list, account },
        {
          onSuccess: () => {
            onDone();
            notice.inPlace(
              t("client:lists.members.added", { name: isolate(accountName(account)), list: isolate(list.name) }),
            );
          },
        },
      );
    },
    addingId: add.isPending ? add.variables.account.id : undefined,
    addError: (account) => (add.isError && add.variables.account.id === account.id ? addFailure(add.error) : undefined),
    remove: (account, onDone) => {
      add.reset();
      remove.mutate(
        { list, account },
        {
          onSuccess: () => {
            onDone();
            notice.inPlace(
              t("client:lists.members.removed", { name: isolate(accountName(account)), list: isolate(list.name) }),
            );
          },
        },
      );
    },
    removingId: remove.isPending ? remove.variables.account.id : undefined,
    removeError: remove.isError
      ? t("client:lists.members.remove_failed", { reason: errorMessage(remove.error) })
      : undefined,
  };
}
