// SPDX-License-Identifier: AGPL-3.0-only
// The controls that the room's page and the mini-player share (ADR-0100 §5, §6; ADR-0101 §7): the call's state, the
// microphone (only a speaker has it; the browser's refusal says why), the sound that the browser blocked, and leaving,
// where the host of a room with others in it gives the next host or ends the room, and who is talking ("{name} is
// talking"). Null while there is no call.
import { useQuery } from "@tanstack/react-query";
import { useLocation } from "@tanstack/react-router";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useViewer } from "../../session";
import type { VoiceLeaveChoice } from "../api/call";
import { type VoiceRoom, type VoiceRoomParticipant, voiceRoomPath, voiceRoomQueryOptions } from "../api/voice";
import { callEndText, useVoiceCallState, type VoiceCallNoticeView } from "./useVoiceCallProvider";
import type { VoiceCall, VoiceCallSnapshot, VoiceMicError } from "./voiceCall";

export type VoiceCallControls = {
  /** The call's room as the call knows it. */
  room: VoiceRoom;
  phase: VoiceCallSnapshot["phase"];
  isSpeaker: boolean;
  micOn: boolean;
  micPending: boolean;
  micError: VoiceMicError | undefined;
  /** Why the server did not take the microphone, localized. */
  micRefused: string | undefined;
  setMicrophone: (on: boolean) => void;
  playbackBlocked: boolean;
  resumePlayback: () => void;
  /**
   * The participants talking now, in the order of the room's participants (the user too, while the microphone
   * sends); empty while nobody talks. The call changes it only when who talks changes, not on each sample.
   */
  talking: readonly VoiceRoomParticipant[];
  /** Whether leaving asks the host for the next host or the end of the room. */
  mustChoose: boolean;
  /** The participants who may become the host when the user, the host, leaves. */
  nextHosts: readonly VoiceRoomParticipant[];
  leave: (choice: VoiceLeaveChoice) => void;
  isLeaving: boolean;
  leaveError: string | undefined;
  resetLeave: () => void;
  /** The last change that others made to the user in the call, for the notice of the dock or the mini-player. */
  notice: VoiceCallNoticeView | undefined;
  dismissNotice: () => void;
};

export function useVoiceCallControls(): VoiceCallControls | null {
  const state = useVoiceCallState();
  const viewer = useViewer();
  const viewerId = viewer.status === "signed-in" ? viewer.account.id : "";
  const [micRefused, setMicRefused] = useState<string | undefined>(undefined);
  const { call, snapshot } = state;
  if (call === null || snapshot === undefined || snapshot.phase === "ended") return null;
  const room = snapshot.room;
  const self =
    room.participants.find((p) => p.session === call.session) ??
    room.participants.find((p) => p.account.id === viewerId);
  const isHost = self?.host ?? false;
  const others = room.participants.filter((p) => p !== self);
  return {
    room,
    phase: snapshot.phase,
    isSpeaker: self?.role === "speaker",
    micOn: snapshot.micOn,
    micPending: snapshot.micPending,
    micError: snapshot.micError,
    micRefused,
    setMicrophone: (on) => {
      call.setMicrophone(on).then(
        () => {
          setMicRefused(undefined);
        },
        (error: unknown) => {
          setMicRefused(errorMessage(error));
        },
      );
    },
    playbackBlocked: snapshot.playbackBlocked,
    resumePlayback: () => {
      void call.resumePlayback();
    },
    talking:
      snapshot.talking.size === 0
        ? []
        : room.participants.filter((p) => p.session !== undefined && snapshot.talking.has(p.session)),
    mustChoose: isHost && others.length > 0,
    nextHosts: others,
    leave: state.leave,
    isLeaving: state.isLeaving,
    leaveError: state.leaveError,
    resetLeave: state.resetLeave,
    notice: state.callNotice,
    dismissNotice: state.dismissCallNotice,
  };
}

/** A path without its trailing slashes, as the router matches it. */
function normalizedPath(pathname: string): string {
  return pathname.length > 1 ? pathname.replace(/\/+$/u, "") : pathname;
}

/**
 * The mini-player's controls, or null while there is no call or the page shows the call itself: the room's page of
 * the call, when it shows the room (not its summary or an error), has the call's controls.
 */
export function useVoiceCallBar(): VoiceCallControls | null {
  const api = useApi();
  const controls = useVoiceCallControls();
  const roomId = controls?.room.id ?? "";
  const pathname = useLocation({ select: (location) => normalizedPath(location.pathname) });
  // Only what the page has read: the room's page shows the call while it has the room.
  const pageRoom = useQuery({ ...voiceRoomQueryOptions(api, roomId), enabled: false });
  if (controls === null) return null;
  if (pathname === voiceRoomPath(roomId) && pageRoom.data !== undefined) return null;
  return controls;
}

/** How the user's call ended without them leaving it, for the notice in the mini-player's place. */
export type VoiceCallEndNotice = {
  /** The call that ended: the notice shows until it is dismissed for it. */
  call: VoiceCall;
  room: VoiceRoom;
  /** The sentence of how it ended ("The room ended: the host ended it"). */
  text: string;
};

/**
 * The end of the user's call (the room ended, they were removed, the connection was lost, they joined from another
 * device), or null while a call goes on, after they left it themselves, or on the room's page, whose join dock says how
 * it ended. On any other page the mini-player went away with the call, so this notice takes its place.
 */
export function useVoiceCallEndNotice(): VoiceCallEndNotice | null {
  const t = useT();
  const { call, snapshot } = useVoiceCallState();
  const pathname = useLocation({ select: (location) => normalizedPath(location.pathname) });
  if (call === null || snapshot?.phase !== "ended" || snapshot.end === undefined) return null;
  if (pathname === voiceRoomPath(call.roomId)) return null;
  return { call, room: snapshot.room, text: callEndText(t, snapshot.end) };
}
