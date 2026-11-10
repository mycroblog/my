// SPDX-License-Identifier: AGPL-3.0-only
// Answering a follow request from its notification (FR-302): the same calls and cache changes as the settings' list of
// follow requests (features/settings), so that the list, the viewer's count of the requests, and the profile agree without
// a reload (web-client/README.md S1). The answers are offered only while the request waits (the relationship's
// `requested_by`); a request that the server no longer has (a 404: answered elsewhere, or withdrawn) is shown as that,
// not as an error. A failure says why in a toast and keeps the answers (E2).
import { useNotice, useToast } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { isolate, nameOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { useRelationship } from "../../profile";
import { useViewer } from "../../session";
import {
  cacheFollowRequestAnswer,
  type FollowRequestAnswer,
  followRequestMutation,
  forgetFollowRequest,
} from "../../settings";
import type { NotificationAccount } from "../api/notifications";

/** What became of a follow request answered from the row: accepted, rejected, or no longer there to answer. */
export type FollowRequestOutcome = "accepted" | "rejected" | "gone";

export type FollowRequestAnswerState =
  /** Nothing to answer: the relationship is loading or failed, or the request does not wait (any more). */
  | { kind: "none" }
  /** The request waits; `pending` is the answer on its way to the server, or null. */
  | { kind: "waiting"; pending: FollowRequestAnswer | null; answer: (answer: FollowRequestAnswer) => void }
  /** The row answered it, or found it gone. */
  | { kind: "answered"; outcome: FollowRequestOutcome };

/** The follow request of `account` to the viewer, and answering it. */
export function useFollowRequestAnswer(account: NotificationAccount): FollowRequestAnswerState {
  // The notification may be newer than the cached relationship: read it again when the row mounts.
  const relationship = useRelationship(account, { fresh: true });
  const [outcome, setOutcome] = useState<FollowRequestOutcome | null>(null);
  const accept = useAnswerMutation(account, "accept", setOutcome);
  const reject = useAnswerMutation(account, "reject", setOutcome);
  const pending = accept.isPending ? "accept" : reject.isPending ? "reject" : null;

  if (outcome !== null) return { kind: "answered", outcome };
  // While an answer is on its way the cache may hold the relationship after it already: the answers stay until it is done.
  if (pending === null && (relationship.kind !== "other" || !relationship.relationship.requested_by))
    return { kind: "none" };
  return {
    kind: "waiting",
    pending,
    answer: (choice) => {
      if (pending === null) (choice === "accept" ? accept : reject).mutate(account.id);
    },
  };
}

/**
 * The mutation of one answer. It is done once what the answer changes is in the cache (it stays pending until then), so
 * that the settings' list and the profile opened next show it at once (S1).
 */
function useAnswerMutation(
  account: NotificationAccount,
  answer: FollowRequestAnswer,
  setOutcome: (outcome: FollowRequestOutcome) => void,
) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const toast = useToast();
  const viewer = useViewer();
  const name = isolate(nameOf(account));
  return useMutation({
    ...followRequestMutation(api, answer),
    onSuccess: async (relationship) => {
      if (viewer.status === "signed-in")
        await cacheFollowRequestAnswer(queryClient, api, answer, relationship, viewer.account.id);
      setOutcome(answer === "accept" ? "accepted" : "rejected");
      notice.inPlace(
        answer === "accept"
          ? t("client:notifications.follow_request.accepted", { name })
          : t("client:notifications.follow_request.rejected", { name }),
      );
    },
    onError: async (error) => {
      if (error instanceof ApiError && error.status === 404) {
        await forgetFollowRequest(queryClient, account.id);
        setOutcome("gone");
        notice.inPlace(t("client:notifications.follow_request.gone"));
        return;
      }
      toast.show({
        message: t("client:notifications.follow_request.failed", { reason: errorMessage(error) }),
        tone: "danger",
      });
    },
  });
}
