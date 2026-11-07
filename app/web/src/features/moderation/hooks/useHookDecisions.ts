// SPDX-License-Identifier: AGPL-3.0-only
// The decisions of the external reviews on the screen: their labels, the list and one decision, the summary, cancelling
// one, and the authors' appeals of refused posts, the list and one appeal, with their review.
import {
  useMutation,
  useMutationState,
  useQueryClient,
  useSuspenseInfiniteQuery,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { useToast } from "@my/ui";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { isAppealDecided } from "../api/appeals";
import {
  decideHookAppealMutation,
  type HookAppeal,
  type HookAppealDecision,
  type HookAppealFilter,
  hookAppealQueryOptions,
  hookAppealsQueryOptions,
  type HookAppealState,
  type HookDecision,
  type HookDecisionFilter,
  type HookDecisionMode,
  hookDecisionQueryOptions,
  hookDecisionsQueryOptions,
  type HookMode,
  type HookName,
  type HookOutcome,
  type HookReason,
  hookSummaryQueryOptions,
  overrideHookDecisionMutation,
  refreshHookAppeals,
  refreshHookDecisions,
} from "../api/hookDecisions";
import { refreshAuditLog } from "../api/staff";
import { type PagedPages, toPagedList } from "./pagedList";

export const hookNameLabels: Readonly<Record<HookName, ClientMessageKey>> = {
  "inbound.status": "client:admin.hook_decisions.hook.inbound_status",
  "inbound.follow": "client:admin.hook_decisions.hook.inbound_follow",
  "signup.check": "client:admin.hook_decisions.hook.signup_check",
  "media.check": "client:admin.hook_decisions.hook.media_check",
  "local.status": "client:admin.hook_decisions.hook.local_status",
  "inbound.actor": "client:admin.hook_decisions.hook.inbound_actor",
};

export const hookOutcomeLabels: Readonly<Record<HookOutcome, ClientMessageKey>> = {
  allow: "client:admin.hook_decisions.outcome.allow",
  drop: "client:admin.hook_decisions.outcome.drop",
  flag: "client:admin.hook_decisions.outcome.flag",
  mark_sensitive: "client:admin.hook_decisions.outcome.mark_sensitive",
  require_approval: "client:admin.hook_decisions.outcome.require_approval",
  reject: "client:admin.hook_decisions.outcome.reject",
  timeout: "client:admin.hook_decisions.outcome.timeout",
  error: "client:admin.hook_decisions.outcome.error",
  invalid: "client:admin.hook_decisions.outcome.invalid",
  breaker_open: "client:admin.hook_decisions.outcome.breaker_open",
};

export const hookReasonLabels: Readonly<Record<HookReason, ClientMessageKey>> = {
  spam: "client:admin.hook_decisions.reason.spam",
  harassment: "client:admin.hook_decisions.reason.harassment",
  sexual: "client:admin.hook_decisions.reason.sexual",
  violence: "client:admin.hook_decisions.reason.violence",
  illegal: "client:admin.hook_decisions.reason.illegal",
  malware: "client:admin.hook_decisions.reason.malware",
  other: "client:admin.hook_decisions.reason.other",
};

export const hookModeLabels: Readonly<Record<HookMode | HookDecisionMode, ClientMessageKey>> = {
  off: "client:admin.hook_decisions.mode.off",
  observe: "client:admin.hook_decisions.mode.observe",
  enforce: "client:admin.hook_decisions.mode.enforce",
};

export function useHookDecisions(filter: HookDecisionFilter): PagedPages<HookDecision> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(hookDecisionsQueryOptions(api, filter)));
}

/** One decision with its cancellation, for its page. */
export function useHookDecision(id: string): HookDecision {
  const api = useApi();
  return useSuspenseQuery(hookDecisionQueryOptions(api, id)).data;
}

export function useHookSummary() {
  const api = useApi();
  return useSuspenseQuery(hookSummaryQueryOptions(api)).data;
}

export type OverrideHookDecision = {
  /** Cancels the decision; `onOverridden` says so, and a failure is shown in a toast. */
  override: (decision: HookDecision) => void;
  /** The IDs of the decisions being cancelled. */
  pendingIds: readonly string[];
};

/**
 * Cancelling decisions. The lists and the audit log are fetched again, then `onOverridden` says what happened. Several
 * may be cancelled at once, so everything after a success is in the mutation's own callbacks.
 */
export function useOverrideHookDecision(onOverridden: (decision: HookDecision) => void): OverrideHookDecision {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const options = overrideHookDecisionMutation(api);
  const mutation = useMutation({
    ...options,
    onSuccess: async (decision) => {
      await Promise.all([refreshAuditLog(queryClient), refreshHookDecisions(queryClient)]);
      onOverridden(decision);
    },
    // The decision stands, and its row and focus stay (web-client/README.md E2).
    onError: (error) => {
      toast.show({
        message: t("client:admin.hook_decisions.override_failed", { reason: errorMessage(error) }),
        tone: "danger",
      });
    },
  });
  const pendingIds = useMutationState({
    filters: { mutationKey: options.mutationKey, status: "pending" },
    select: (pending) => pending.state.variables as string,
  });
  return {
    override: (decision) => {
      mutation.mutate(decision.id);
    },
    pendingIds,
  };
}

export const hookAppealStateLabels: Readonly<Record<HookAppealState, ClientMessageKey>> = {
  pending: "client:admin.hook_appeals.state.pending",
  upheld: "client:admin.hook_appeals.state.upheld",
  overturned: "client:admin.hook_appeals.state.overturned",
};

/** The appeals of refused posts of a filter, newest first. */
export function useHookAppeals(filter: HookAppealFilter): PagedPages<HookAppeal> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(hookAppealsQueryOptions(api, filter)));
}

/** One appeal of a refused post, for its page. */
export function useHookAppeal(id: string): HookAppeal {
  const api = useApi();
  return useSuspenseQuery(hookAppealQueryOptions(api, id)).data;
}

/** The decision being confirmed, of which appeal. */
type HookAppealConfirmation = { appeal: HookAppeal; decision: HookAppealDecision };

/** The confirmation dialog: open, or closing and still showing the last confirmation while it fades out. */
type HookAppealDialog = { isOpen: boolean; confirmation: HookAppealConfirmation } | null;

/** What the appeals of refused posts need to decide one. */
export type HookAppealReview = {
  /** Opens the confirmation of `decision` on `appeal`. */
  open: (appeal: HookAppeal, decision: HookAppealDecision) => void;
  isOpen: boolean;
  /** The confirmation open now or last, which a closing dialog still shows; null before the first one. */
  confirmation: HookAppealConfirmation | null;
  /** Sends the decision; on success the confirmation closes and the page shows the appeal decided. */
  confirm: () => void;
  /** Closes the confirmation without deciding. */
  cancel: () => void;
  isPending: boolean;
  /** Why the decision failed, localized, for the confirmation (which stays open to try again). */
  error: string | undefined;
  /** What the last decision did, for the section's StatusMessage; undefined before the first one. */
  status: string | undefined;
};

/**
 * Deciding the appeals of refused posts. A decision cannot be changed, so it is confirmed first (NFR-MOD-7). Once the
 * appeals, the decisions (an overturn cancels one) and the audit log are fetched again, the confirmation closes, the
 * status says what the decision did, and `onDecided()` is called: the page's buttons are gone, and the page moves focus
 * to the section's heading. An appeal another moderator decided the other way first (409 appeal-decided)
 * is read again with the lists, and the status says so.
 */
export function useHookAppealReview(onDecided: () => void): HookAppealReview {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [dialog, setDialog] = useState<HookAppealDialog>(null);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const refresh = () =>
    Promise.all([refreshAuditLog(queryClient), refreshHookAppeals(queryClient), refreshHookDecisions(queryClient)]);
  const mutation = useMutation({
    ...decideHookAppealMutation(api),
    onSuccess: async (appeal) => {
      await refresh();
      setDialog((current) => current && { ...current, isOpen: false });
      setStatus(
        t(appeal.state === "overturned" ? "client:admin.hook_appeals.overturned" : "client:admin.hook_appeals.upheld"),
      );
      onDecided();
    },
    onError: async (error) => {
      if (!isAppealDecided(error)) return;
      await refresh();
      setDialog((current) => current && { ...current, isOpen: false });
      setStatus(t("client:admin.hook_appeals.already_decided"));
      onDecided();
    },
  });
  const isOpen = dialog?.isOpen ?? false;
  return {
    open: (appeal, decision) => {
      mutation.reset();
      setDialog({ isOpen: true, confirmation: { appeal, decision } });
    },
    isOpen,
    confirmation: dialog?.confirmation ?? null,
    confirm: () => {
      if (dialog === null || !dialog.isOpen || mutation.isPending) return;
      const { appeal, decision } = dialog.confirmation;
      mutation.mutate({ id: appeal.id, decision });
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
