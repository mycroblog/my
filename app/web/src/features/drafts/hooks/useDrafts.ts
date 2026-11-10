// SPDX-License-Identifier: AGPL-3.0-only
// The drafts page (web-client/screens/drafts.md): one list of drafts, scheduled or not, and what its rows do. Opening a
// draft reads it again for its ETag and puts it in the new-post form (the compose feature), asking first when the form
// holds something else. Cancelling a schedule, moving it, and deleting a draft are sent at once; each marks both lists
// stale and says what happened once those on screen are fetched again (S1).
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheDraftChange,
  deleteDraft,
  draftsQueryOptions,
  getDraft,
  isDraftGone,
  scheduleDraft,
  type ServerDraft,
  unscheduleDraft,
  useOpenDraft,
} from "../../compose";

type DraftVersion = Awaited<ReturnType<typeof getDraft>>;

/** A row action on its way: which draft, and what. */
export type PendingAction = { id: string; action: "edit" | "unschedule" | "delete" };

export type DraftList = {
  items: ServerDraft[];
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => void;
  pending: PendingAction | null;
  /** Opens the draft in the form, or asks first (`replacing`) when the form holds something else. */
  edit: (draft: ServerDraft) => void;
  /** The draft read to open while the form holds something else: the page asks whether to replace it. */
  replacing: DraftVersion | null;
  /** Answers the question: open the draft in place of what the form holds, or not. */
  confirmReplace: (answer: boolean) => void;
  /** Moves the schedule of `draft` to `at`: resolves to null once moved, or to the localized reason it is not. */
  reschedule: (draft: ServerDraft, at: Date) => Promise<string | null>;
  /** Cancels the schedule of `draft`; `onDone` is called once it is off this list. */
  unschedule: (draft: ServerDraft, onDone: () => void) => void;
  /** Deletes `draft`; resolves to null once it is off the list, or to the localized reason it is not (the dialog shows it). */
  remove: (draft: ServerDraft, onDone: () => void) => Promise<string | null>;
};

/** The localized reason a row action failed: a draft gone from the server says so. */
function failure(t: ReturnType<typeof useT>, error: unknown): string {
  return isDraftGone(error) ? t("client:drafts.gone") : errorMessage(error);
}

/** The scheduled drafts (`scheduled`) or the others, newest first, and what their rows do. */
export function useDrafts(scheduled: boolean): DraftList {
  const api = useApi();
  const t = useT();
  const intl = useIntl();
  const notice = useNotice();
  const queryClient = useQueryClient();
  const compose = useOpenDraft();
  const query = useSuspenseInfiniteQuery(draftsQueryOptions(api, scheduled));
  const [replacing, setReplacing] = useState<DraftVersion | null>(null);

  const open = useMutation({ mutationFn: (id: string) => getDraft(api, id) });
  const unschedule = useMutation({ mutationFn: (id: string) => unscheduleDraft(api, id) });
  const remove = useMutation({ mutationFn: (id: string) => deleteDraft(api, id) });
  const reschedule = useMutation({
    mutationFn: ({ id, at }: { id: string; at: Date }) => scheduleDraft(api, id, at),
  });

  const pending: PendingAction | null = open.isPending
    ? { id: open.variables, action: "edit" }
    : unschedule.isPending
      ? { id: unschedule.variables, action: "unschedule" }
      : remove.isPending
        ? { id: remove.variables, action: "delete" }
        : null;

  return {
    items: query.data.pages.flatMap((page) => page.items),
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => {
      // A refetch in progress would overwrite the page fetched now (TanStack Query, "Infinite Queries").
      if (!query.isFetching) void query.fetchNextPage();
    },
    pending,
    edit: (draft) => {
      if (compose === null) return;
      open.mutate(draft.id, {
        onSuccess: (version) => {
          if (compose.isWriting()) setReplacing(version);
          else compose.openDraft(version);
        },
        onError: (error) => {
          notice.inPlace(failure(t, error), "assertive");
        },
      });
    },
    replacing,
    confirmReplace: (answer) => {
      if (answer && replacing !== null) compose?.openDraft(replacing);
      setReplacing(null);
    },
    reschedule: async (draft, at) => {
      try {
        const moved = await reschedule.mutateAsync({ id: draft.id, at });
        await cacheDraftChange(queryClient);
        const date = intl.formatDate(moved.scheduled_at ?? at, { dateStyle: "medium", timeStyle: "short" });
        notice.inPlace(t("client:drafts.schedule.scheduled", { date }));
        return null;
      } catch (error) {
        return failure(t, error);
      }
    },
    unschedule: (draft, onDone) => {
      unschedule.mutate(draft.id, {
        onSuccess: () => {
          // The row's place is noted before the list is fetched again without it (useFocusAfterRemoval).
          onDone();
          void cacheDraftChange(queryClient).then(() => {
            notice.inPlace(t("client:drafts.unscheduled"));
          });
        },
        // A failure keeps the row and its focus, and says why (web-client/README.md E2).
        onError: (error) => {
          notice.inPlace(failure(t, error), "assertive");
        },
      });
    },
    remove: async (draft, onDone) => {
      try {
        await remove.mutateAsync(draft.id);
        onDone();
        await cacheDraftChange(queryClient);
        notice.inPlace(t("client:drafts.deleted"));
        return null;
      } catch (error) {
        return failure(t, error);
      }
    },
  };
}
