// SPDX-License-Identifier: AGPL-3.0-only
// The notification policy of the notification settings (ADR-0158 §10, web-client/screens/settings-notifications.md):
// one radio group per condition that saves its action as soon as it changes, shows the action being saved meanwhile
// (web-client/README.md S8), and has a save mark of its own. A save the server refuses puts the action back and says
// why under the section.
import { type SaveState, useNotice } from "@my/ui";
import { useMutation, useMutationState, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheNotificationPolicy,
  type NotificationPolicyAction,
  type NotificationPolicyChange,
  type NotificationPolicyCondition,
  notificationPolicyConditions,
  notificationPolicyQueryOptions,
  updateNotificationPolicyMutation,
} from "../api/notificationPolicy";
import { saveStateOf } from "./saveState";

/** One condition of the policy, as its radio group shows it. */
type NotificationPolicyRow = {
  condition: NotificationPolicyCondition;
  /** The action shown: the one being saved, or the saved one. */
  action: NotificationPolicyAction;
  setAction: (action: NotificationPolicyAction) => void;
  saveState: SaveState;
};

export type NotificationPolicyView = {
  rows: readonly NotificationPolicyRow[];
  /** Why the last save failed, localized; undefined while it did not. */
  error: string | undefined;
};

/** The user's notification policy, which the route's loader has loaded (it suspends until then). */
export function useNotificationPolicy(): NotificationPolicyView {
  const api = useApi();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const t = useT();
  const policy = useSuspenseQuery(notificationPolicyQueryOptions(api)).data;
  const options = updateNotificationPolicyMutation(api);
  const mutation = useMutation({
    ...options,
    // One change at a time, in the order chosen, so that the last answer is the last choice.
    scope: { id: "notification_policy" },
    onSuccess: (saved) => {
      cacheNotificationPolicy(api, queryClient, saved);
      // The save mark of the group shows it.
      notice.inPlace(t("client:settings.saved"));
    },
  });
  // Every change waiting or on its way, and every save since the page opened, the last one last.
  const saves = useMutationState({
    filters: { mutationKey: options.mutationKey },
    select: (m) => ({ change: m.state.variables as NotificationPolicyChange, status: m.state.status }),
  });
  const rows = notificationPolicyConditions.map((condition): NotificationPolicyRow => {
    const last = saves.findLast((save) => save.change.condition === condition);
    return {
      condition,
      action: last?.status === "pending" ? last.change.action : policy[condition],
      setAction: (action) => {
        mutation.mutate({ condition, action });
      },
      saveState:
        last === undefined
          ? "idle"
          : saveStateOf({
              isPending: last.status === "pending",
              isSuccess: last.status === "success",
              isError: last.status === "error",
            }),
    };
  });
  return { rows, error: mutation.isError ? errorMessage(mutation.error) : undefined };
}
