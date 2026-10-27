// SPDX-License-Identifier: AGPL-3.0-only
// The call as a room's page shows it (ADR-0101 §2–§7; ADR-0100 §5, §6, §10): the app's call (VoiceCallProvider) when
// it is this room's, with the user's operations that only the page has (speaking and listening, a raised hand,
// reactions); otherwise the way to join, and how the last call of this room ended. Joining here leaves the call of
// another room.
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useViewer } from "../../session";
import type { ReactionChoice } from "../../emoji";
import { reactInVoiceRoom, type VoiceSelfOperation, voiceSelfOperation } from "../api/call";
import { cacheVoiceRoom, type VoiceRoom, type VoiceRoomParticipant } from "../api/voice";
import { voiceSupported } from "../rtc/connect";
import { useVoiceCallState } from "./useVoiceCallProvider";
import type { VoiceCallEnd, VoiceCallSnapshot, VoiceReactionShown } from "./voiceCall";

export type VoiceCallControl =
  | {
      status: "outside";
      /** Whether this browser has WebRTC, Web Audio, and the microphone. */
      supported: boolean;
      /** Whether the user is in the room from another device or tab. */
      inRoomElsewhere: boolean;
      join: () => void;
      isJoining: boolean;
      joinError: string | undefined;
      /** How the last call of this room ended. */
      end: VoiceCallEnd | undefined;
    }
  | {
      status: "in-call";
      phase: VoiceCallSnapshot["phase"];
      self: VoiceRoomParticipant | undefined;
      isSpeaker: boolean;
      isHost: boolean;
      /** Whether the user hosts or co-hosts, who handles the participants. */
      manages: boolean;
      operate: (operation: VoiceSelfOperation) => void;
      /** The operation waiting for the server. */
      pendingOperation: VoiceSelfOperation | undefined;
      react: (choice: ReactionChoice) => void;
      /** Why the last operation or reaction failed, localized. */
      actionError: string | undefined;
      /** The microphones of the participants (account ID → on). */
      mics: ReadonlyMap<string, boolean>;
      /** The reactions showing now, by participant. */
      reactions: ReadonlyMap<string, readonly VoiceReactionShown[]>;
      /**
       * The sessions talking now: a participant talks when `talking.has(participant.session)` (the user too, while the
       * microphone sends). The set changes, and the page renders again, only when who talks changes.
       */
      talking: ReadonlySet<string>;
    };

export function useVoiceCall(room: VoiceRoom): VoiceCallControl {
  const api = useApi();
  const queryClient = useQueryClient();
  const viewer = useViewer();
  const state = useVoiceCallState();
  const [actionError, setActionError] = useState<string | undefined>(undefined);
  const viewerId = viewer.status === "signed-in" ? viewer.account.id : "";
  const call = state.call !== null && state.call.roomId === room.id ? state.call : null;
  const snapshot = call === null ? undefined : state.snapshot;

  const operation = useMutation({
    mutationKey: ["voice", "self", room.id] as const,
    mutationFn: (op: VoiceSelfOperation) => voiceSelfOperation(api, room.id, op),
    onSuccess: async (updated) => {
      setActionError(undefined);
      // The call takes the answer at once, so that the microphone works before the next `state`.
      call?.applyRoom(updated);
      await cacheVoiceRoom(api, queryClient, updated);
    },
    onError: (error) => {
      setActionError(errorMessage(error));
    },
  });

  const reaction = useMutation({
    mutationKey: ["voice", "reaction", room.id] as const,
    mutationFn: (emoji: string) => reactInVoiceRoom(api, room.id, emoji),
    onSuccess: () => {
      setActionError(undefined);
    },
    onError: (error) => {
      setActionError(errorMessage(error));
    },
  });

  if (call === null || snapshot === undefined || snapshot.phase === "ended") {
    const end = snapshot?.end;
    return {
      status: "outside",
      supported: voiceSupported(),
      inRoomElsewhere: call === null && room.participants.some((p) => p.account.id === viewerId) && state.call?.roomId !== room.id,
      join: () => {
        state.join(room.id, viewerId);
      },
      isJoining: state.joiningRoom === room.id,
      joinError: state.joinError?.roomId === room.id ? state.joinError.message : undefined,
      end,
    };
  }
  const self = room.participants.find((p) => p.session === call.session) ?? room.participants.find((p) => p.account.id === viewerId);
  const reactions = new Map<string, VoiceReactionShown[]>();
  for (const shown of snapshot.reactions) reactions.set(shown.participant, [...(reactions.get(shown.participant) ?? []), shown]);
  const isHost = self?.host ?? false;
  return {
    status: "in-call",
    phase: snapshot.phase,
    self,
    isSpeaker: self?.role === "speaker",
    isHost,
    manages: isHost || (self?.cohost ?? false),
    operate: (op) => {
      operation.mutate(op);
    },
    pendingOperation: operation.isPending ? operation.variables : undefined,
    react: (choice) => {
      reaction.mutate("unicode" in choice ? choice.unicode : `:${choice.custom.shortcode}:`);
    },
    actionError,
    mics: snapshot.mics,
    reactions,
    talking: snapshot.talking,
  };
}
