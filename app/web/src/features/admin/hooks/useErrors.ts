// SPDX-License-Identifier: AGPL-3.0-only
// The errors screens (ADR-0103 §8): the error groups under a filter, one group, resolving, muting and unmuting it,
// and copying the IDs that find its latest occurrence in the logs and the traces.
import { useToast } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type PagedPages, toPagedList } from "../../moderation";
import {
  cacheErrorGroupChange,
  type ErrorFilter,
  type ErrorGroup,
  type ErrorGroupAction,
  errorGroupActionMutation,
  type ErrorGroupDetail,
  errorGroupQueryOptions,
  errorGroupsQueryOptions,
} from "../api/errors";

/** The error groups under `filter`, the most recently seen first. */
export function useErrorGroups(filter: ErrorFilter): PagedPages<ErrorGroup> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(errorGroupsQueryOptions(api, filter)));
}

/** One error group with its latest occurrence, its hourly counts, and the changes of its state. */
export function useErrorGroup(id: string): ErrorGroupDetail {
  const api = useApi();
  return useSuspenseQuery(errorGroupQueryOptions(api, id)).data;
}

const doneMessages: Readonly<Record<ErrorGroupAction, ClientMessageKey>> = {
  resolve: "client:admin.errors.resolved",
  mute: "client:admin.errors.muted",
  unmute: "client:admin.errors.unmuted",
};

export type ErrorGroupActions = {
  /** Resolves, mutes or unmutes the group; a failure is shown in a toast, and the group and its buttons stay. */
  run: (action: ErrorGroupAction) => void;
  /** The action being sent, if any. */
  pending: ErrorGroupAction | undefined;
  /** What the last action did, for the section's StatusMessage. */
  status: string | undefined;
};

/**
 * Resolving, muting and unmuting the group `id`. The buttons change with the state, so `onChanged` is called once the
 * change is sent (to move focus, useFocusAfterChange); the result is said once the page shows the new state.
 */
export function useErrorGroupActions(id: string, onChanged: () => void): ErrorGroupActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...errorGroupActionMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (group, { action }) => {
      onChanged();
      await cacheErrorGroupChange(api, queryClient, group);
      setStatus(t(doneMessages[action]));
    },
    // The group keeps its state; the failure is shown and the buttons stay (web-client/README.md E2).
    onError: (error) => {
      toast.show({ message: t("client:admin.errors.action_failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });
  return {
    run: (action) => {
      mutation.mutate({ id, action });
    },
    pending: mutation.isPending ? mutation.variables.action : undefined,
    status,
  };
}

/** Copies a value (a request ID or a trace ID) to the clipboard and says whether that worked in a toast. */
export function useCopyErrorValue(): (value: string) => void {
  const t = useT();
  const toast = useToast();
  return (value) => {
    navigator.clipboard.writeText(value).then(
      () => {
        // The toast is announced too.
        toast.show({ message: t("client:admin.errors.copied"), tone: "success" });
      },
      () => {
        toast.show({ message: t("client:admin.errors.copy_failed"), tone: "danger" });
      },
    );
  };
}
