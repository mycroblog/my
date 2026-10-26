// SPDX-License-Identifier: AGPL-3.0-only
// What the host and the co-hosts do in a call (ADR-0100 §5, §7): on a participant, let them speak, dismiss a raised
// hand, stop them speaking, mute them, remove them (they do not join again), and, for the host only, make or stop
// them being a co-host and hand the room over; and invite a local user as a listener or a speaker. The host and the
// co-hosts do not operate on each other beyond that, and nobody on the host; the menu offers only what the server
// allows. Each result is announced.
import { useAnnouncer } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { inviteToVoiceRoom, type VoiceParticipantOperation, voiceParticipantOperation, type VoiceRole } from "../api/call";
import { cacheVoiceRoom, type VoiceRoom, type VoiceRoomParticipant } from "../api/voice";
import { participantName } from "./voiceCall";

/** The operations that the user may run on `participant`, in the order of its menu. */
export function participantOperations(self: VoiceRoomParticipant | undefined, participant: VoiceRoomParticipant): VoiceParticipantOperation[] {
  if (self === undefined || participant.account.id === self.account.id || participant.host) return [];
  if (!self.host && !self.cohost) return [];
  // A co-host does not operate on another co-host.
  if (!self.host && participant.cohost) return [];
  const ops: VoiceParticipantOperation[] = [];
  if (participant.role === "listener") ops.push("grant");
  if (participant.hand_raised) ops.push("dismiss");
  if (participant.role === "speaker") ops.push("revoke", "mute");
  if (!participant.cohost) ops.push("remove");
  if (self.host) ops.push(participant.cohost ? "uncohost" : "cohost", "hand-over");
  return ops;
}

export type VoiceModeration = {
  operate: (participant: VoiceRoomParticipant, operation: VoiceParticipantOperation) => void;
  /** The operation waiting for the server, with its participant. */
  pending: { accountId: string; operation: VoiceParticipantOperation } | undefined;
  error: string | undefined;
};

export function useVoiceModeration(room: VoiceRoom): VoiceModeration {
  const api = useApi();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const t = useT();
  const mutation = useMutation({
    mutationKey: ["voice", "moderate", room.id] as const,
    mutationFn: ({ participant, operation }: { participant: VoiceRoomParticipant; operation: VoiceParticipantOperation }) =>
      voiceParticipantOperation(api, room.id, participant.account.id, operation),
    onSuccess: async (updated, { participant, operation }) => {
      if (updated !== undefined) await cacheVoiceRoom(api, queryClient, updated);
      const name = participantName(participant);
      announce(
        operation === "grant"
          ? t("client:voice.moderate.done.grant", { name })
          : operation === "dismiss"
            ? t("client:voice.moderate.done.dismiss", { name })
            : operation === "revoke"
              ? t("client:voice.moderate.done.revoke", { name })
              : operation === "mute"
                ? t("client:voice.moderate.done.mute", { name })
                : operation === "remove"
                  ? t("client:voice.moderate.done.remove", { name })
                  : operation === "cohost"
                    ? t("client:voice.moderate.done.cohost", { name })
                    : operation === "uncohost"
                      ? t("client:voice.moderate.done.uncohost", { name })
                      : t("client:voice.moderate.done.hand_over", { name }),
      );
    },
  });
  return {
    operate: (participant, operation) => {
      mutation.mutate({ participant, operation });
    },
    pending: mutation.isPending ? { accountId: mutation.variables.participant.account.id, operation: mutation.variables.operation } : undefined,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
  };
}

export type VoiceInvite = {
  username: string;
  setUsername: (username: string) => void;
  role: VoiceRole;
  setRole: (role: VoiceRole) => void;
  /** Sends the invitation; `close` closes the dialog after it. */
  submit: (close: () => void) => void;
  isPending: boolean;
  error: string | undefined;
  /** Starts the form again when the dialog opens. */
  reset: () => void;
};

/** The form that invites a local user to the room. */
export function useVoiceInvite(room: VoiceRoom): VoiceInvite {
  const api = useApi();
  const announce = useAnnouncer();
  const t = useT();
  const [username, setUsername] = useState("");
  const [role, setRole] = useState<VoiceRole>("listener");
  const mutation = useMutation({
    mutationKey: ["voice", "invite", room.id] as const,
    mutationFn: ({ name, as }: { name: string; as: VoiceRole; close: () => void }) => inviteToVoiceRoom(api, room.id, name, as),
    onSuccess: (invitation, { close }) => {
      announce(t("client:voice.invite.sent", { name: invitation.account.display_name || invitation.account.username }));
      close();
    },
  });
  return {
    username,
    setUsername,
    role,
    setRole,
    submit: (close) => {
      if (username.trim() === "") return;
      mutation.mutate({ name: username, as: role, close });
    },
    isPending: mutation.isPending,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
    reset: () => {
      setUsername("");
      setRole("listener");
      mutation.reset();
    },
  };
}
