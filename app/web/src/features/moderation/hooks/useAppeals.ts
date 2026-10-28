// SPDX-License-Identifier: AGPL-3.0-only
// The review of appeals (FR-707): the appeals of a tab, newest first; one appeal with its action; and deciding
// it through its confirmation, with an optional explanation that the user who appealed is shown. A decision
// cannot be changed afterwards, so it asks first (NFR-MOD-7); an overturn lifts the action, which the page shows
// at once from the appeal that the decision answers.
import { useMutation, useQueryClient, useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, type Translate, useT } from "../../../lib/i18n";
import {
  type Appeal,
  type AppealDecision,
  type AppealFilter,
  appealQueryOptions,
  appealsQueryOptions,
  type AppealState,
  decideAppealMutation,
  isAppealDecided,
  refreshDecidedAppeal,
  storeAppeal,
} from "../api/appeals";
import { type PagedPages, toPagedList } from "./pagedList";
import { accountActionKindLabels } from "./useReports";

export const appealStateLabels: Readonly<Record<AppealState, ClientMessageKey>> = {
  pending: "client:admin.appeals.state.pending",
  upheld: "client:admin.appeals.state.upheld",
  overturned: "client:admin.appeals.state.overturned",
};

/** The appeals of a tab, newest first. */
export function useAppeals(filter: AppealFilter): PagedPages<Appeal> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(appealsQueryOptions(api, filter)));
}

/** One appeal, which the route has loaded. */
export function useAppeal(id: string): Appeal {
  const api = useApi();
  return useSuspenseQuery(appealQueryOptions(api, id)).data;
}

/** What an appeal is of, for a heading or a sentence: "Appeal of Suspend". */
export function appealTitle(t: Translate, appeal: Appeal): string {
  return t("client:admin.appeals.of", { kind: t(accountActionKindLabels[appeal.action.kind]) });
}

/** What a decision did, in words: an overturn says whether it lifted the action, or found it ended already. */
function decidedMessage(t: Translate, appeal: Appeal): string {
  if (appeal.state === "upheld") return t("client:admin.appeal.upheld");
  return appeal.action.state === "lifted"
    ? t("client:admin.appeal.overturned")
    : t("client:admin.appeal.overturned_ended");
}

/** The decision being confirmed and the explanation typed so far. */
type Confirmation = { decision: AppealDecision; reason: string };

/** The confirmation dialog: open, or closing and still showing the last confirmation while it fades out. */
type DialogState = { isOpen: boolean; confirmation: Confirmation } | null;

/** What the decision region of an appeal's page needs. */
export type AppealReview = {
  /** Opens the confirmation of `decision`. */
  open: (decision: AppealDecision) => void;
  isOpen: boolean;
  /** The confirmation open now or last, which a closing dialog still shows; null before the first one. */
  confirmation: Confirmation | null;
  changeReason: (reason: string) => void;
  /** Sends the decision; on success the confirmation closes and the page shows the decided appeal. */
  confirm: () => void;
  /** Closes the confirmation without deciding. */
  cancel: () => void;
  isPending: boolean;
  /** Why the decision failed, localized, for the confirmation (which stays open to try again). */
  error: string | undefined;
  /** What the last decision did, for the region's StatusMessage; undefined before the first one. */
  status: string | undefined;
};

/**
 * Deciding one appeal. `onDecided()` is called once the cache has the appeal after it and the lists of appeals
 * are fetched again (the decision stays pending until then), when the region's buttons go away (the page moves
 * focus to the region's heading). An appeal that another moderator decided the other way first (409
 * appeal-decided) is read again with the lists, then closes the confirmation, and the status says so.
 */
export function useAppealReview(id: string, onDecided: () => void): AppealReview {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [dialog, setDialog] = useState<DialogState>(null);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...decideAppealMutation(api),
    onSuccess: async (appeal) => {
      // The decision marks the lists stale, and is done once those on screen are fetched again (the confirmation
      // stays pending until then); going back to a list fetches it again first, so it shows the appeal as it is
      // now, not as it was (refreshAppealLists, web-client/README.md S7). The
      // confirmation closes as the page shows the decision, and focus then moves to the region's heading.
      await storeAppeal(queryClient, api, appeal);
      setDialog((current) => current && { ...current, isOpen: false });
      setStatus(decidedMessage(t, appeal));
      onDecided();
    },
    onError: async (error) => {
      if (!isAppealDecided(error)) return;
      // The other moderator's decision changed the lists as well as the appeal.
      await refreshDecidedAppeal(queryClient, api, id);
      setDialog((current) => current && { ...current, isOpen: false });
      setStatus(t("client:admin.appeal.already_decided"));
      onDecided();
    },
  });
  const isOpen = dialog?.isOpen ?? false;
  return {
    open: (decision) => {
      mutation.reset();
      setDialog({ isOpen: true, confirmation: { decision, reason: "" } });
    },
    isOpen,
    confirmation: dialog?.confirmation ?? null,
    changeReason: (reason) => {
      if (dialog !== null) setDialog({ ...dialog, confirmation: { ...dialog.confirmation, reason } });
    },
    confirm: () => {
      if (dialog === null || !dialog.isOpen || mutation.isPending) return;
      const { decision, reason } = dialog.confirmation;
      mutation.mutate({ id, decision, reason: reason.trim() });
    },
    cancel: () => {
      if (dialog !== null) setDialog({ ...dialog, isOpen: false });
      mutation.reset();
    },
    isPending: mutation.isPending,
    error: isOpen && mutation.isError && !isAppealDecided(mutation.error) ? errorMessage(mutation.error) : undefined,
    status,
  };
}
