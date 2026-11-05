// SPDX-License-Identifier: AGPL-3.0-only
// Appealing a post that the server's automated review refused (ADR-0147): the author sends the text they tried to post,
// which starts as the text sent and can be edited, and an optional comment for the moderators, in an alert dialog. An
// appeal cannot be changed once sent, and it does not post anything: a moderator who overturns the decision lets the
// author post again. What came of it replaces the appeal's button in the form's notice of the refusal.
import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { appealHookDecision, type ClosedHookAppealProblem, closedHookAppeal } from "../api/compose";
import type { Rejection } from "./draftStore";
import { composeTextId } from "./useCompose";

/** What each problem of an appeal that cannot be sent says, in the client's words (web-client/README.md E2). */
const closedMessages: Readonly<Record<ClosedHookAppealProblem, ClientMessageKey>> = {
  "/api/v1/problems/already-appealed": "client:compose.rejected.appeal.already_appealed",
  "/api/v1/problems/appeal-window-closed": "client:compose.rejected.appeal.window_closed",
  "/api/v1/problems/not-appealable": "client:compose.rejected.appeal.not_appealable",
};

/** The appeal dialog: open, or closing and still showing what was typed while it fades out. */
type DialogState = { isOpen: boolean; text: string; comment: string } | null;

export type RejectionAppeal = {
  /** Opens the appeal with the text that was sent and no comment. */
  open: () => void;
  isOpen: boolean;
  text: string;
  changeText: (text: string) => void;
  comment: string;
  changeComment: (comment: string) => void;
  /** Sends the appeal; on success the dialog closes and the notice says it was sent. */
  submit: () => void;
  /** Closes the dialog without appealing. */
  cancel: () => void;
  isPending: boolean;
  /** Why the appeal failed, localized, for the dialog (which stays open to try again). */
  error: string | undefined;
};

/**
 * The appeal of `rejection`. `settle` records what came of it: sent, or refused because the decision can no longer be
 * appealed (appealed already, its window closed, or not appealable), which no change to the appeal helps. Either way
 * the dialog closes, the button that opened it gives way to the outcome, and focus goes back to the post's text.
 */
export function useRejectionAppeal(
  rejection: Rejection,
  settle: (outcome: NonNullable<Rejection["outcome"]>) => void,
): RejectionAppeal {
  const api = useApi();
  const t = useT();
  const [dialog, setDialog] = useState<DialogState>(null);
  const close = (outcome: NonNullable<Rejection["outcome"]>) => {
    setDialog((current) => current && { ...current, isOpen: false });
    settle(outcome);
    // The dialog's trigger is gone with the outcome; after React Aria has tried to return focus there, focus goes to
    // the draft, which the author may post again.
    requestAnimationFrame(() => document.getElementById(composeTextId)?.focus());
  };
  const mutation = useMutation({
    mutationFn: ({ id, text, comment }: { id: string; text: string; comment: string }) =>
      appealHookDecision(api, id, { status_text: text, ...(comment === "" ? {} : { comment }) }),
    onSuccess: () => {
      close({ message: t("client:compose.rejected.appeal.sent"), sent: true });
    },
    onError: (error) => {
      const closed = closedHookAppeal(error);
      if (closed !== null) close({ message: t(closedMessages[closed]), sent: false });
    },
  });
  const isOpen = dialog?.isOpen ?? false;
  return {
    open: () => {
      mutation.reset();
      setDialog({ isOpen: true, text: rejection.text, comment: "" });
    },
    isOpen,
    text: dialog?.text ?? "",
    changeText: (text) => {
      if (dialog !== null) setDialog({ ...dialog, text });
    },
    comment: dialog?.comment ?? "",
    changeComment: (comment) => {
      if (dialog !== null) setDialog({ ...dialog, comment });
    },
    submit: () => {
      if (dialog === null || !dialog.isOpen || mutation.isPending || rejection.hookDecisionId === null) return;
      mutation.mutate({ id: rejection.hookDecisionId, text: dialog.text, comment: dialog.comment.trim() });
    },
    cancel: () => {
      if (dialog !== null) setDialog({ ...dialog, isOpen: false });
      mutation.reset();
    },
    isPending: mutation.isPending,
    error:
      isOpen && mutation.isError && closedHookAppeal(mutation.error) === null
        ? errorMessage(mutation.error)
        : undefined,
  };
}
