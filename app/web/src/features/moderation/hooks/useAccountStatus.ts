// SPDX-License-Identifier: AGPL-3.0-only
// The user's own account status (FR-715) and appealing an action on it (FR-707): the actions in effect now,
// the past ones (expired or lifted), the posts an action on posts acted on, and the appeal, which the user
// writes in an alert dialog. An appeal's answer is the action with the appeal, which replaces the action in
// both lists at once, so the form gives way to the appeal's state on the page.
import { useMutation, useQueries, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { type Status, statusQueryOptions } from "../../status";
import {
  type ActionScope,
  appealActionMutation,
  cacheAppealedAction,
  isStaleAppealError,
  type ModerationAction,
  ownActionsQueryOptions,
  refreshOwnActions,
} from "../api/actions";

/** What a list of actions needs: the actions so far, and loading more. */
export type ActionList = {
  items: ModerationAction[];
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => void;
};

/**
 * The actions of `scope`, newest first; `keep` leaves out the others. The past actions are the whole list
 * without those in effect: the API filters only for the actions in effect.
 */
function useActionList(scope: ActionScope, keep: (action: ModerationAction) => boolean): ActionList {
  const api = useApi();
  const query = useSuspenseInfiniteQuery(ownActionsQueryOptions(api, scope));
  return {
    items: query.data.pages.flatMap((page) => page.items).filter(keep),
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => {
      // A fetch in progress would be overwritten by the page fetched now (TanStack Query, "Infinite Queries").
      if (!query.isFetching) void query.fetchNextPage();
    },
  };
}

/** The actions in effect on the user's account now, newest first. */
export function useActiveActions(): ActionList {
  return useActionList("active", () => true);
}

/** The actions that ended on the user's account (expired or lifted), newest first. */
export function usePastActions(): ActionList {
  return useActionList("all", (action) => action.state !== "active");
}

/** One post an action acted on: loading, shown, or not visible any more (deleted, as `delete_statuses` does). */
export type ActionStatus =
  | { id: string; status: "loading" }
  | { id: string; status: "found"; value: Status }
  | { id: string; status: "missing" };

/** The posts that an action on posts acted on, in its order. */
export function useActionStatuses(statusIds: readonly string[]): ActionStatus[] {
  const api = useApi();
  // A deleted post is a 404, which trying again does not change.
  const results = useQueries({ queries: statusIds.map((id) => ({ ...statusQueryOptions(api, id), retry: false })) });
  return statusIds.map((id, index) => {
    const result = results[index];
    if (result?.data !== undefined) return { id, status: "found", value: result.data };
    return result?.isError === true ? { id, status: "missing" } : { id, status: "loading" };
  });
}

/** The id of the heading of an action's appeal, which focus moves to once the appeal is sent. */
export function appealHeadingId(actionId: string): string {
  return `appeal-${actionId}`;
}

/** The appeal dialog: open, or closing and still showing its action while it fades out. */
type DialogState = { isOpen: boolean; action: ModerationAction; text: string } | null;

/** What the account status page needs to appeal its actions. */
export type AppealForm = {
  /** Opens the appeal of `action`, with an empty text. */
  open: (action: ModerationAction) => void;
  isOpen: boolean;
  /** The action being appealed now or last, which a closing dialog still shows; null before the first one. */
  action: ModerationAction | null;
  text: string;
  changeText: (text: string) => void;
  /** Sends the appeal; on success the dialog closes, and focus moves to the appeal's heading. */
  submit: () => void;
  /** Closes the dialog without appealing. */
  cancel: () => void;
  isPending: boolean;
  /** Why the appeal failed, localized, for the dialog (which stays open). */
  error: string | undefined;
  /** What the last appeal did, for the page's StatusMessage; undefined before the first one. */
  status: string | undefined;
};

/**
 * Appealing the actions on the user's account. An appeal refused because the action cannot be appealed any
 * more (appealed already, its window closed) reads the lists again, which take the action's form away.
 */
export function useAppealForm(): AppealForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [dialog, setDialog] = useState<DialogState>(null);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const focusRef = useRef<string | null>(null);

  // Runs after every render: focus moves in the DOM, outside React, once the appeal's heading is there. The
  // button that opened the dialog is gone with the form, so the dialog cannot return focus to it.
  useEffect(() => {
    const actionId = focusRef.current;
    if (actionId === null) return;
    const heading = document.getElementById(appealHeadingId(actionId));
    if (heading === null) return;
    focusRef.current = null;
    heading.focus();
  });

  const mutation = useMutation({
    ...appealActionMutation(api),
    onSuccess: (action) => {
      setDialog((current) => current && { ...current, isOpen: false });
      cacheAppealedAction(api, queryClient, action);
      focusRef.current = action.id;
      setStatus(t("client:account_status.appeal.sent", { kind: action.kind_label }));
    },
    onError: (error) => {
      if (isStaleAppealError(error)) void refreshOwnActions(queryClient);
    },
  });
  const isOpen = dialog?.isOpen ?? false;
  return {
    open: (action) => {
      mutation.reset();
      setDialog({ isOpen: true, action, text: "" });
    },
    isOpen,
    action: dialog?.action ?? null,
    text: dialog?.text ?? "",
    changeText: (text) => {
      if (dialog !== null) setDialog({ ...dialog, text });
    },
    submit: () => {
      if (dialog === null || !dialog.isOpen || mutation.isPending) return;
      mutation.mutate({ actionId: dialog.action.id, text: dialog.text.trim() });
    },
    cancel: () => {
      if (dialog !== null) setDialog({ ...dialog, isOpen: false });
      mutation.reset();
    },
    isPending: mutation.isPending,
    error: isOpen && mutation.isError ? errorMessage(mutation.error) : undefined,
    status,
  };
}
