// SPDX-License-Identifier: AGPL-3.0-only
// The decisions of the external reviews on the screen: their labels, the list, the summary, and cancelling one.
import {
  useMutation,
  useMutationState,
  useQueryClient,
  useSuspenseInfiniteQuery,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { useToast } from "@my/ui";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  type HookDecision,
  type HookDecisionFilter,
  type HookDecisionMode,
  hookDecisionsQueryOptions,
  type HookMode,
  type HookName,
  type HookOutcome,
  hookSummaryQueryOptions,
  overrideHookDecisionMutation,
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

export const hookModeLabels: Readonly<Record<HookMode | HookDecisionMode, ClientMessageKey>> = {
  off: "client:admin.hook_decisions.mode.off",
  observe: "client:admin.hook_decisions.mode.observe",
  enforce: "client:admin.hook_decisions.mode.enforce",
};

export function useHookDecisions(filter: HookDecisionFilter): PagedPages<HookDecision> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(hookDecisionsQueryOptions(api, filter)));
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
