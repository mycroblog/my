// SPDX-License-Identifier: AGPL-3.0-only
// The "Add to list" dialog of a followed account's profile (web-client/screens/profile.md §4): the user's lists, each
// checked while the account is in it, and checking or unchecking one saves at once. The hook returns no JSX.
import { useNotice } from "@my/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  type Account,
  accountListsQueryOptions,
  addListMemberMutation,
  cacheMemberChange,
  type List,
  listFailure,
  listsQueryOptions,
  maxListMembers,
  removeListMemberMutation,
} from "../api/lists";
import { isolate, useLimits } from "./names";

/** One list in the dialog. */
export type ListChoice = {
  list: List;
  /** Whether the account is in the list, or is being put in it (the check shows the change while it is saved). */
  checked: boolean;
  /** Whether the change of this list is being saved. */
  isPending: boolean;
  /** Whether the list is full, so that the account cannot be put in it (an unchecked list only). */
  isFull: boolean;
  /** Why the last change of this list failed, localized, shown beside its check; undefined when it did not. */
  error: string | undefined;
};

export type AddToList =
  | { status: "loading" }
  | { status: "error"; message: string; retry: () => void }
  | { status: "done"; choices: readonly ListChoice[]; toggle: (list: List, checked: boolean) => void };

export function useAddToList(account: Account): AddToList {
  const api = useApi();
  const t = useT();
  const limits = useLimits();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const lists = useQuery(listsQueryOptions(api));
  const memberOf = useQuery(accountListsQueryOptions(api, account.id));
  // The last failure, by list, until that list changes again.
  const [failed, setFailed] = useState<{ id: string; message: string } | null>(null);
  // The cache is updated before the mutation settles, so that the check does not go back for a moment.
  const add = useMutation({
    ...addListMemberMutation(api),
    onSuccess: (change) => cacheMemberChange(api, queryClient, change, 1),
  });
  const remove = useMutation({
    ...removeListMemberMutation(api),
    onSuccess: (change) => cacheMemberChange(api, queryClient, change, -1),
  });
  if (lists.isError || memberOf.isError)
    return {
      status: "error",
      message: errorMessage(lists.error ?? memberOf.error),
      retry: () => {
        if (lists.isError) void lists.refetch();
        if (memberOf.isError) void memberOf.refetch();
      },
    };
  if (lists.data === undefined || memberOf.data === undefined) return { status: "loading" };
  const ids = new Set(memberOf.data.items.map((list) => list.id));
  const pending = (list: List) =>
    (add.isPending && add.variables.list.id === list.id) || (remove.isPending && remove.variables.list.id === list.id);
  const choices = lists.data.items.map((list): ListChoice => {
    const isPending = pending(list);
    // While a change is saved, the check shows where it goes (web-client/README.md S8).
    let checked = ids.has(list.id);
    if (isPending) checked = add.isPending && add.variables.list.id === list.id;
    return {
      list,
      checked,
      isPending,
      isFull: !checked && list.members_count >= maxListMembers,
      error: failed?.id === list.id ? failed.message : undefined,
    };
  });
  const failure = (list: List, error: unknown) => {
    const message =
      listFailure(error) === "too-many-members"
        ? t("client:lists.members.limit", { max: limits.members })
        : t("client:lists.add_to_list.failed", { reason: errorMessage(error) });
    setFailed({ id: list.id, message });
    notice.inPlace(message, "assertive");
  };
  return {
    status: "done",
    choices,
    toggle: (list, checked) => {
      if (pending(list)) return;
      setFailed(null);
      const mutation = checked ? add : remove;
      mutation.mutate(
        { list, account },
        {
          onSuccess: () => {
            notice.inPlace(
              t(checked ? "client:lists.add_to_list.added" : "client:lists.add_to_list.removed", {
                list: isolate(list.name),
              }),
            );
          },
          onError: (error) => {
            failure(list, error);
          },
        },
      );
    },
  };
}
