// SPDX-License-Identifier: AGPL-3.0-only
// The call of a room's page (ADR-0101 §2–§7; ADR-0100 §5, §6, §10): joining starts the user's session in the room
// (VoiceCall: heartbeats, the room stream, the audio, the renewal of the grant), which lasts while the page shows; the
// user's operations (the microphone, speaking and listening, a raised hand, reactions, leaving) go to the server, and
// what changes for the user is announced (web-client/README.md Y7). Leaving the page during a call asks first:
// the call ends with the page (the host chooses the next host or ends the room). The end of the call (the room
// ended, the user was removed or joined from another device) stays on the page with its reason.
import { useAnnouncer } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useBlocker } from "@tanstack/react-router";
import { useEffect, useState, useSyncExternalStore } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useViewer } from "../../session";
import type { ReactionChoice } from "../../emoji";
import {
  cacheVoiceRoomState,
  leaveVoiceRoom,
  reactInVoiceRoom,
  readVoiceStream,
  refreshVoiceGrant,
  refreshVoiceRoom,
  sendVoiceHeartbeat,
  sendVoiceSignal,
  setVoiceMic,
  type VoiceLeaveChoice,
  type VoiceSelfOperation,
  voiceSelfOperation,
} from "../api/call";
import { cacheVoiceRoom, joinVoiceRoomMutation, type VoiceRoom, type VoiceRoomParticipant } from "../api/voice";
import { browserEnvironment, voiceSupported } from "../rtc/connect";
import { voiceEndReasonLabel } from "./useVoiceHistory";
import { participantName, VoiceCall, type VoiceCallEnd, type VoiceCallNotice, type VoiceCallSnapshot, type VoiceReactionShown } from "./voiceCall";

export type VoiceCallControl =
  | {
      status: "outside";
      /** Whether this browser has WebRTC and the microphone. */
      supported: boolean;
      /** Whether the user is in the room from another device or tab. */
      inRoomElsewhere: boolean;
      join: () => void;
      isJoining: boolean;
      joinError: string | undefined;
      /** How the previous call of this page ended. */
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
      micOn: boolean;
      micFailed: boolean;
      setMicrophone: (on: boolean) => void;
      playbackBlocked: boolean;
      resumePlayback: () => void;
      operate: (operation: VoiceSelfOperation) => void;
      /** The operation waiting for the server. */
      pendingOperation: VoiceSelfOperation | undefined;
      react: (choice: ReactionChoice) => void;
      /** Why the last operation, reaction, or microphone change failed, localized. */
      actionError: string | undefined;
      /** The microphones of the participants (account ID → on). */
      mics: ReadonlyMap<string, boolean>;
      /** The reactions showing now, by participant. */
      reactions: ReadonlyMap<string, readonly VoiceReactionShown[]>;
      /** The participants who may become the host when the user, the host, leaves. */
      nextHosts: readonly VoiceRoomParticipant[];
      /** Whether leaving asks the host for the next host or the end of the room. */
      mustChoose: boolean;
      leave: (choice: VoiceLeaveChoice) => void;
      isLeaving: boolean;
      leaveError: string | undefined;
      /** A navigation away waits until the user leaves the room or stays. */
      isLeavingPage: boolean;
      stay: () => void;
    };

const idleSnapshot: VoiceCallSnapshot = { phase: "ended", end: undefined, micOn: false, micFailed: false, playbackBlocked: false, mics: new Map(), reactions: [] };

const noSubscription = () => () => undefined;

const idleSnapshotOf = () => idleSnapshot;

/** The words announced for a notice of the call. */
function noticeText(t: ReturnType<typeof useT>, notice: VoiceCallNotice): string {
  switch (notice.kind) {
    case "speaker":
      return t("client:voice.notice.speaker");
    case "listener":
      return t("client:voice.notice.listener");
    case "muted":
      return t("client:voice.notice.muted");
    case "hand-down":
      return t("client:voice.notice.hand_down");
    case "cohost":
      return t("client:voice.notice.cohost");
    case "cohost-removed":
      return t("client:voice.notice.cohost_removed");
    case "host":
      return t("client:voice.notice.host");
    case "reconnecting":
      return t("client:voice.notice.reconnecting");
    case "reconnected":
      return t("client:voice.notice.reconnected");
    case "hand-raised":
      return t("client:voice.notice.hand_raised", { name: participantName(notice.participant) });
  }
}

export function useVoiceCall(room: VoiceRoom): VoiceCallControl {
  const api = useApi();
  const queryClient = useQueryClient();
  const viewer = useViewer();
  const announce = useAnnouncer();
  const t = useT();
  const id = room.id;
  const accountId = viewer.status === "signed-in" ? viewer.account.id : "";
  const [call, setCall] = useState<VoiceCall | null>(null);
  const [actionError, setActionError] = useState<string | undefined>(undefined);
  const snapshot = useSyncExternalStore(call?.subscribe ?? noSubscription, call?.getSnapshot ?? idleSnapshotOf);
  const inCall = call !== null && snapshot.phase !== "ended";

  // The call is an external system that lasts while the page shows it (react.dev, "Synchronizing with Effects").
  useEffect(() => {
    if (call === null) return;
    call.start();
    return () => {
      call.close();
    };
  }, [call]);

  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) => inCall && current.pathname !== next.pathname,
    enableBeforeUnload: inCall,
    withResolver: true,
  });

  const join = useMutation({
    ...joinVoiceRoomMutation(api, id),
    onSuccess: async (joined) => {
      await cacheVoiceRoom(api, queryClient, joined.room);
      setActionError(undefined);
      const next: VoiceCall = new VoiceCall({
        accountId,
        join: joined,
        environment: browserEnvironment(),
        onNotice: (notice) => {
          announce(noticeText(t, notice));
        },
        server: {
          join: () => joinVoiceRoomMutation(api, id).mutationFn(),
          heartbeat: (session) => sendVoiceHeartbeat(api, id, session),
          refreshGrant: (session) => refreshVoiceGrant(api, id, session),
          sendSignal: (session, signal) => sendVoiceSignal(api, id, session, signal),
          setMic: (session, on) => setVoiceMic(api, id, session, on),
          openStream: (session, options) => readVoiceStream(api, id, session, options),
          onRoom: (state) => {
            cacheVoiceRoomState(api, queryClient, state);
          },
          onEnd: () => {
            const end = next.getSnapshot().end;
            if (end !== undefined) announce(callEndText(t, end), "assertive");
            void refreshVoiceRoom(api, queryClient, id);
          },
        },
      });
      setCall(next);
    },
  });

  const operation = useMutation({
    mutationKey: ["voice", "self", id] as const,
    mutationFn: (op: VoiceSelfOperation) => voiceSelfOperation(api, id, op),
    onSuccess: async (updated) => {
      setActionError(undefined);
      await cacheVoiceRoom(api, queryClient, updated);
    },
    onError: (error) => {
      setActionError(errorMessage(error));
    },
  });

  const reaction = useMutation({
    mutationKey: ["voice", "reaction", id] as const,
    mutationFn: (emoji: string) => reactInVoiceRoom(api, id, emoji),
    onSuccess: () => {
      setActionError(undefined);
    },
    onError: (error) => {
      setActionError(errorMessage(error));
    },
  });

  const leave = useMutation({
    mutationKey: ["voice", "leave", id] as const,
    mutationFn: (choice: VoiceLeaveChoice) => leaveVoiceRoom(api, id, choice),
    onSuccess: async () => {
      call?.close();
      setCall(null);
      await refreshVoiceRoom(api, queryClient, id);
      blocker.proceed?.();
    },
  });

  const viewerId = accountId;
  if (!inCall) {
    const end = call === null ? undefined : snapshot.end;
    return {
      status: "outside",
      supported: voiceSupported(),
      inRoomElsewhere: room.participants.some((p) => p.account.id === viewerId) && end === undefined,
      join: () => {
        join.mutate();
      },
      isJoining: join.isPending,
      joinError: join.isError ? errorMessage(join.error) : undefined,
      end,
    };
  }
  const self = room.participants.find((p) => p.session === call.session) ?? room.participants.find((p) => p.account.id === viewerId);
  const others = room.participants.filter((p) => p.account.id !== viewerId);
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
    micOn: snapshot.micOn,
    micFailed: snapshot.micFailed,
    setMicrophone: (on) => {
      call.setMicrophone(on).then(
        () => {
          setActionError(undefined);
        },
        (error: unknown) => {
          setActionError(errorMessage(error));
        },
      );
    },
    playbackBlocked: snapshot.playbackBlocked,
    resumePlayback: () => {
      void call.resumePlayback();
    },
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
    nextHosts: others,
    mustChoose: isHost && others.length > 0,
    leave: (choice) => {
      leave.mutate(choice);
    },
    isLeaving: leave.isPending,
    leaveError: leave.isError ? errorMessage(leave.error) : undefined,
    isLeavingPage: blocker.status === "blocked",
    stay: () => {
      leave.reset();
      blocker.reset?.();
    },
  };
}

/** The sentence of how a call ended. */
export function callEndText(t: ReturnType<typeof useT>, end: VoiceCallEnd): string {
  switch (end.kind) {
    case "ended":
      return t("client:voice.call.ended", { reason: t(voiceEndReasonLabel(end.reason)) });
    case "replaced":
      return t("client:voice.call.replaced");
    case "lost":
      return t("client:voice.call.lost");
    case "unsupported":
      return t("client:voice.call.unsupported");
    case "removed":
      return end.reason === "moved"
        ? t("client:voice.call.moved")
        : end.reason === "disconnected"
          ? t("client:voice.call.disconnected")
          : end.reason === "suspended"
            ? t("client:voice.call.suspended")
            : end.reason === "left"
              ? t("client:voice.call.left")
              : end.reason === "blocked"
                ? t("client:voice.call.blocked")
                : t("client:voice.call.removed");
  }
}
