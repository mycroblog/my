// SPDX-License-Identifier: AGPL-3.0-only
// A moderator ending the voice chat room that a report points at (ADR-0100 §11, manage_reports): the confirmation
// with an optional reason for the moderators, then the room ends for everyone in it. Once the report, the audit log
// and the voice chat queries on screen are fetched again, the confirmation closes and the page says what happened;
// a failure stays in the confirmation with the reason kept (web-client/README.md E2).
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { endVoiceRoomMutation, refreshEndedVoiceRoom, type ReportDetail } from "../api/reports";

export type EndVoiceRoom = {
  isOpen: boolean;
  open: () => void;
  /** Closes the confirmation without ending the room; the next opening starts without a reason. */
  cancel: () => void;
  reason: string;
  changeReason: (reason: string) => void;
  /** Ends the room (the dialog has checked the reason's length). */
  confirm: () => void;
  isPending: boolean;
  /** Why ending the room failed, localized. */
  error: string | undefined;
  /** What the last end did, for the region that reports it. */
  status: string | undefined;
};

export function useEndVoiceRoom(report: Pick<ReportDetail, "id" | "target_account_id">, roomId: string): EndVoiceRoom {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [isOpen, setIsOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...endVoiceRoomMutation(api),
    onSuccess: async () => {
      await refreshEndedVoiceRoom(queryClient, api, report);
      setIsOpen(false);
      setReason("");
      setStatus(t("client:admin.report.voice.ended"));
    },
  });
  return {
    isOpen,
    open: () => {
      mutation.reset();
      setStatus(undefined);
      setIsOpen(true);
    },
    cancel: () => {
      setIsOpen(false);
      setReason("");
    },
    reason,
    changeReason: setReason,
    confirm: () => {
      if (!mutation.isPending) mutation.mutate({ roomId, reason: reason.trim() });
    },
    isPending: mutation.isPending,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
    status,
  };
}
