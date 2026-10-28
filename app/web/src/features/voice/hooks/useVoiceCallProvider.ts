// SPDX-License-Identifier: AGPL-3.0-only
// The user's call as a state of the whole app (ADR-0101 §2–§7; #1345 answer 45: the call goes on across pages, with a
// small mini-player on the screen's edge, as in X's Spaces). VoiceCallProvider holds at most one call: joining a room
// starts it (from the join button's gesture, which starts the audio, ADR-0101 §7), and so does opening one, which joins
// the host (ADR-0100 §5; from the gesture of the form's submit button); joining another room replaces it,
// and leaving, the end of the room, or signing out ends it. Its requests go through the app's
// API client, the rooms it reads go into the Query cache, and what changes for the user is announced in the app's live
// region (web-client/README.md Y7). While a call lasts, closing or reloading the tab asks first (beforeunload), a
// page that goes away leaves the room with a request that outlives it (pagehide), signing out leaves it before the
// token is revoked, and the maintenance or the closure of the server, whose screens replace the pages, end it.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createContext, use, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useApi, useSession, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { hasClosed, useClosure } from "../../../lib/closure";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { type Maintenance, useMaintenance } from "../../../lib/maintenance";
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
import { cacheVoiceRoom, joinVoiceRoomMutation, type VoiceRoomJoin } from "../api/voice";
import { browserEnvironment } from "../rtc/connect";
import type { VoiceEnvironment } from "../rtc/types";
import { voiceEndReasonLabel } from "./useVoiceHistory";
import {
  participantName,
  VoiceCall,
  type VoiceCallEnd,
  type VoiceCallNotice,
  type VoiceCallSnapshot,
} from "./voiceCall";

/** What a call needs from the user's gesture, taken before a request that joins the user to a room (`prepare`). */
export type VoiceCallGesture = { accountId: string; environment: VoiceEnvironment; generation: number };

export type VoiceCallState = {
  /** The call: the one going on, or the last one, which has ended (its snapshot says how). */
  call: VoiceCall | null;
  snapshot: VoiceCallSnapshot | undefined;
  /** Whether a call goes on. */
  inCall: boolean;
  /** Joins the room `roomId` as the account `accountId` from the user's gesture, leaving the call of another room. */
  join: (roomId: string, accountId: string) => void;
  /**
   * Takes what a call needs from the user's gesture (it starts the audio, ADR-0101 §7), for another request that joins
   * the user to a room: opening a room joins its host.
   */
  prepare: (accountId: string) => VoiceCallGesture;
  /** Starts the call of `joined`, the answer of a request that joined the user, with the gesture taken before it. */
  adopt: (joined: VoiceRoomJoin, gesture: VoiceCallGesture) => void;
  /** The room being joined. */
  joiningRoom: string | undefined;
  joinError: { roomId: string; message: string } | undefined;
  /** Leaves the call's room; the host of a room with others in it gives the next host or ends it. */
  leave: (choice: VoiceLeaveChoice) => void;
  isLeaving: boolean;
  leaveError: string | undefined;
  /** Forgets the failure of the last leave (the dialog closed). */
  resetLeave: () => void;
  /**
   * The last change that others made to the user in the call ("The host turned your microphone off"), for the notice
   * of the dock or the mini-player (web-client/16-look-states-and-wording.md G25); undefined for none.
   */
  callNotice: VoiceCallNoticeView | undefined;
  dismissCallNotice: () => void;
};

/** A notice of the call as it shows: its words, and an id that tells a new notice of the same words apart. */
export type VoiceCallNoticeView = { id: number; text: string };

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

/** The state of VoiceCallProvider; `maintenance` is the app's, whose screen replaces every page while it lasts. */
export function useVoiceCallProvider(maintenance: Maintenance): VoiceCallState {
  const api = useApi();
  const session = useSession();
  const queryClient = useQueryClient();
  const signedIn = useSessionStatus() === "signed-in";
  const notices = useNotice();
  const t = useT();
  // The words of the announcements follow the locale the app shows now, also for a call joined in another one.
  const tRef = useRef(t);
  useEffect(() => {
    tRef.current = t;
  }, [t]);
  const [active, setActive] = useState<VoiceCall | null>(null);
  // The last change that others made to the user in the call, until it is dismissed, another replaces it, or the call ends.
  const [callNotice, setCallNotice] = useState<VoiceCallNoticeView | undefined>(undefined);
  const nextNoticeIdRef = useRef(0);
  // Joins counted by the user: a join that answers after the user changed is not this user's call.
  const userGenerationRef = useRef(0);
  // Once the user signs out (also to switch accounts), the call is over.
  const call = signedIn ? active : null;
  const snapshot = useSyncExternalStore(call?.subscribe ?? noSubscription, call?.getSnapshot ?? noSnapshot);
  const inCall = snapshot !== undefined && snapshot.phase !== "ended";
  // Screens without the pages (and so without the mini-player): the server's maintenance, and its closure for a user.
  const underMaintenance = useMaintenance(maintenance) !== null;
  const closure = useClosure();
  const appDown = underMaintenance || (signedIn && hasClosed(closure));

  // The call is an external system that lasts while the app shows it (react.dev, "Synchronizing with Effects"): it
  // ends when another call replaces it, or the app goes away.
  useEffect(() => {
    if (active === null) return;
    return () => {
      active.close();
    };
  }, [active]);

  /** Starts the call of `joined` (a join, or the opening of a room), unless the user changed since the gesture. */
  const startCall = async (joined: VoiceRoomJoin, { accountId, environment, generation }: VoiceCallGesture) => {
    const roomId = joined.room.id;
    if (generation !== userGenerationRef.current) {
      // The user signed out while joining: that session must not stay in the room.
      void leaveVoiceRoom(api, roomId, undefined).catch(() => undefined);
      return;
    }
    await cacheVoiceRoom(api, queryClient, joined.room);
    const next: VoiceCall = new VoiceCall({
      accountId,
      join: joined,
      environment,
      onNotice: (notice) => {
        // Shown on the dock or the mini-player (callNotice) and read out with it.
        const text = noticeText(tRef.current, notice);
        setCallNotice({ id: nextNoticeIdRef.current++, text });
        notices.inPlace(text);
      },
      server: {
        join: () => joinVoiceRoomMutation(api, roomId).mutationFn(),
        abandon: () => leaveVoiceRoom(api, roomId, undefined),
        heartbeat: (id) => sendVoiceHeartbeat(api, roomId, id),
        refreshGrant: (id) => refreshVoiceGrant(api, roomId, id),
        sendSignal: (id, signal) => sendVoiceSignal(api, roomId, id, signal),
        setMic: (id, on) => setVoiceMic(api, roomId, id, on),
        openStream: (id, options) => readVoiceStream(api, roomId, id, options),
        onRoom: (state) => {
          cacheVoiceRoomState(api, queryClient, state);
        },
        onEnd: () => {
          const end = next.getSnapshot().end;
          setCallNotice(undefined);
          // Shown by the room's page (its join dock) or, on any other page, the mini-player's notice of the end.
          if (end !== undefined) notices.inPlace(callEndText(tRef.current, end), "assertive");
          void refreshVoiceRoom(api, queryClient, roomId);
        },
      },
    });
    // Joining another room left the previous one on the server (moved); its call ends here (the effect closes it).
    next.start();
    setActive(next);
    setCallNotice(undefined);
  };

  const join = useMutation({
    mutationKey: ["voice", "join"] as const,
    mutationFn: ({ roomId }: { roomId: string } & VoiceCallGesture) => joinVoiceRoomMutation(api, roomId).mutationFn(),
    onSuccess: (joined, { accountId, environment, generation }) =>
      startCall(joined, { accountId, environment, generation }),
  });

  const leave = useMutation({
    mutationKey: ["voice", "leave"] as const,
    mutationFn: ({ roomId, choice }: { roomId: string; choice: VoiceLeaveChoice }) =>
      leaveVoiceRoom(api, roomId, choice),
    onSuccess: async (_, { roomId }) => {
      call?.close();
      setActive(null);
      setCallNotice(undefined);
      // The mini-player or the dock goes away, and the room's page shows its join button again.
      notices.inPlace(tRef.current("client:voice.call.left"));
      await refreshVoiceRoom(api, queryClient, roomId);
    },
  });

  // A new user (a sign-in, or the end of the session in this tab or another) starts with no call and no failures of
  // the previous one.
  const { reset: resetJoin } = join;
  const { reset: resetLeave } = leave;
  useEffect(
    () =>
      session.onUserChange(() => {
        userGenerationRef.current++;
        setActive((current) => {
          current?.close();
          return null;
        });
        resetJoin();
        resetLeave();
      }),
    [session, resetJoin, resetLeave],
  );

  useEffect(() => {
    if (!inCall || call === null) return;
    const roomId = call.roomId;
    // Signing out in this tab leaves the room first, while the token still works.
    const stopSignOut = session.onBeforeSignOut(async () => {
      await leaveVoiceRoom(api, roomId, undefined).catch(() => undefined);
      call.close();
    });
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    // A page that goes away (also into the back-forward cache) leaves the room with a request that outlives it.
    const pageHide = () => {
      leaveVoiceRoomOnUnload(api, roomId);
      call.close();
    };
    window.addEventListener("beforeunload", beforeUnload);
    window.addEventListener("pagehide", pageHide);
    return () => {
      stopSignOut();
      window.removeEventListener("beforeunload", beforeUnload);
      window.removeEventListener("pagehide", pageHide);
    };
  }, [inCall, call, api, session]);

  // The maintenance or the closure replaces the pages: the call cannot go on unseen.
  useEffect(() => {
    if (!appDown || !inCall || call === null) return;
    leaveVoiceRoomOnUnload(api, call.roomId);
    call.close();
  }, [appDown, inCall, call, api]);

  return {
    call,
    snapshot,
    inCall,
    join: (roomId, accountId) => {
      // One join at a time: a second press while one runs does nothing.
      if (join.isPending) return;
      // From the gesture of the join button: the audio of the call starts here (ADR-0101 §7).
      join.mutate({ roomId, accountId, environment: browserEnvironment(), generation: userGenerationRef.current });
    },
    prepare: (accountId) => ({ accountId, environment: browserEnvironment(), generation: userGenerationRef.current }),
    adopt: (joined, gesture) => {
      void startCall(joined, gesture);
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
    callNotice: inCall ? callNotice : undefined,
    dismissCallNotice: () => {
      setCallNotice(undefined);
    },
  };
}
