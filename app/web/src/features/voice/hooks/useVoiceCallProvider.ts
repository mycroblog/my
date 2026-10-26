// SPDX-License-Identifier: AGPL-3.0-only
// The user's call as a state of the whole app (ADR-0101 §2–§7; #1345 answer 45: the call goes on across pages, with a
// small call bar on the screen's edge, as in X's Spaces). VoiceCallProvider holds at most one call: joining a room
// starts it (from the join button's gesture, which starts the audio, ADR-0101 §7), joining another room replaces it,
// and leaving, the end of the room, or signing out ends it. Its requests go through the app's
// API client, the rooms it reads go into the Query cache, and what changes for the user is announced in the app's live
// region (web-client/README.md Y7). While a call lasts, closing or reloading the tab asks first (beforeunload), and a
// page that goes away leaves the room with a request that outlives it (pagehide).
import { useAnnouncer } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createContext, use, useEffect, useState, useSyncExternalStore } from "react";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheVoiceRoomState,
  leaveVoiceRoom,
  leaveVoiceRoomOnUnload,
  readVoiceStream,
  refreshVoiceGrant,
  refreshVoiceRoom,
  sendVoiceHeartbeat,
  sendVoiceSignal,
  setVoiceMic,
  type VoiceLeaveChoice,
} from "../api/call";
import { cacheVoiceRoom, joinVoiceRoomMutation } from "../api/voice";
import { browserEnvironment } from "../rtc/connect";
import type { VoiceEnvironment } from "../rtc/types";
import { voiceEndReasonLabel } from "./useVoiceHistory";
import { participantName, VoiceCall, type VoiceCallEnd, type VoiceCallNotice, type VoiceCallSnapshot } from "./voiceCall";

export type VoiceCallState = {
  /** The call: the one going on, or the last one, which has ended (its snapshot says how). */
  call: VoiceCall | null;
  snapshot: VoiceCallSnapshot | undefined;
  /** Whether a call goes on. */
  inCall: boolean;
  /** Joins the room `roomId` as the account `accountId` from the user's gesture, leaving the call of another room. */
  join: (roomId: string, accountId: string) => void;
  /** The room being joined. */
  joiningRoom: string | undefined;
  joinError: { roomId: string; message: string } | undefined;
  /** Leaves the call's room; the host of a room with others in it gives the next host or ends it. */
  leave: (choice: VoiceLeaveChoice) => void;
  isLeaving: boolean;
  leaveError: string | undefined;
  /** Forgets the failure of the last leave (the dialog closed). */
  resetLeave: () => void;
};

export const VoiceCallContext = createContext<VoiceCallState | null>(null);

/** The app's call, from VoiceCallProvider. */
export function useVoiceCallState(): VoiceCallState {
  const state = use(VoiceCallContext);
  if (state === null) throw new Error("voice: useVoiceCallState needs VoiceCallProvider");
  return state;
}

const noSubscription = () => () => undefined;

const noSnapshot = () => undefined;

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
      switch (end.reason) {
        case "moved":
          return t("client:voice.call.moved");
        case "disconnected":
          return t("client:voice.call.disconnected");
        case "suspended":
          return t("client:voice.call.suspended");
        case "left":
          return t("client:voice.call.left");
        case "blocked":
          return t("client:voice.call.blocked");
        default:
          return t("client:voice.call.removed");
      }
  }
}

/** The state of VoiceCallProvider. */
export function useVoiceCallProvider(): VoiceCallState {
  const api = useApi();
  const queryClient = useQueryClient();
  const signedIn = useSessionStatus() === "signed-in";
  const announce = useAnnouncer();
  const t = useT();
  const [active, setActive] = useState<VoiceCall | null>(null);
  // Once the user signs out (also to switch accounts), the call is over.
  const call = signedIn ? active : null;
  const snapshot = useSyncExternalStore(call?.subscribe ?? noSubscription, call?.getSnapshot ?? noSnapshot);
  const inCall = snapshot !== undefined && snapshot.phase !== "ended";

  // The call is an external system that lasts while the app shows it (react.dev, "Synchronizing with Effects"): it
  // ends when another call replaces it, the account changes, or the app goes away.
  useEffect(() => {
    if (active === null) return;
    if (!signedIn) active.close();
    return () => {
      active.close();
    };
  }, [active, signedIn]);

  useEffect(() => {
    if (!inCall || call === null) return;
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    const pageHide = () => {
      leaveVoiceRoomOnUnload(api, call.roomId);
      call.close();
    };
    window.addEventListener("beforeunload", beforeUnload);
    window.addEventListener("pagehide", pageHide);
    return () => {
      window.removeEventListener("beforeunload", beforeUnload);
      window.removeEventListener("pagehide", pageHide);
    };
  }, [inCall, call, api]);

  const join = useMutation({
    mutationKey: ["voice", "join"] as const,
    mutationFn: ({ roomId }: { roomId: string; accountId: string; environment: VoiceEnvironment }) => joinVoiceRoomMutation(api, roomId).mutationFn(),
    onSuccess: async (joined, { roomId, accountId, environment }) => {
      await cacheVoiceRoom(api, queryClient, joined.room);
      const next: VoiceCall = new VoiceCall({
        accountId,
        join: joined,
        environment,
        onNotice: (notice) => {
          announce(noticeText(t, notice));
        },
        server: {
          join: () => joinVoiceRoomMutation(api, roomId).mutationFn(),
          abandon: () => leaveVoiceRoom(api, roomId, undefined),
          heartbeat: (session) => sendVoiceHeartbeat(api, roomId, session),
          refreshGrant: (session) => refreshVoiceGrant(api, roomId, session),
          sendSignal: (session, signal) => sendVoiceSignal(api, roomId, session, signal),
          setMic: (session, on) => setVoiceMic(api, roomId, session, on),
          openStream: (session, options) => readVoiceStream(api, roomId, session, options),
          onRoom: (state) => {
            cacheVoiceRoomState(api, queryClient, state);
          },
          onEnd: () => {
            const end = next.getSnapshot().end;
            if (end !== undefined) announce(callEndText(t, end), "assertive");
            void refreshVoiceRoom(api, queryClient, roomId);
          },
        },
      });
      // Joining another room left the previous one on the server (moved); its call ends here (the effect closes it).
      next.start();
      setActive(next);
    },
  });

  const leave = useMutation({
    mutationKey: ["voice", "leave"] as const,
    mutationFn: ({ roomId, choice }: { roomId: string; choice: VoiceLeaveChoice }) => leaveVoiceRoom(api, roomId, choice),
    onSuccess: async (_, { roomId }) => {
      call?.close();
      setActive(null);
      announce(t("client:voice.call.left"));
      await refreshVoiceRoom(api, queryClient, roomId);
    },
  });

  return {
    call,
    snapshot,
    inCall,
    join: (roomId, accountId) => {
      // From the gesture of the join button: the audio of the call starts here (ADR-0101 §7).
      join.mutate({ roomId, accountId, environment: browserEnvironment() });
    },
    joiningRoom: join.isPending ? join.variables.roomId : undefined,
    joinError: join.isError ? { roomId: join.variables.roomId, message: errorMessage(join.error) } : undefined,
    leave: (choice) => {
      if (call !== null) leave.mutate({ roomId: call.roomId, choice });
    },
    isLeaving: leave.isPending,
    leaveError: leave.isError ? errorMessage(leave.error) : undefined,
    resetLeave: () => {
      leave.reset();
    },
  };
}
