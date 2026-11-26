// SPDX-License-Identifier: AGPL-3.0-only

// What carries the audio of a call (ADR-0025 §2, ADR-0101 §7): one RoomSession per joined session of a room, over
// P2P or my's SFU. It knows nothing of mycroblog, holds no React and no part of the page, and imports no
// other package of the workspace; the caller gives it the grant, the rosters, the signals of P2P, a way to a new grant
// and its environment.

export { browserEnvironment, microphoneConstraints, type BrowserMedia } from "./browser";
export { disconnectWait, offerWait, resendWait } from "./p2p";
export { qualityInterval } from "./quality";
export { createRoomSession, UnsupportedRoomKindError } from "./session";
export { talkingHold, talkingSample, talkingThreshold } from "./talking";
export {
  rtcConfiguration,
  RoomSessionFailure,
  type ConnectionQuality,
  type ConnectionState,
  type IceServer,
  type IncomingSignal,
  type LevelMeter,
  type OutgoingSignal,
  type ParticipantLink,
  type RemoteAudio,
  type RoomConnection,
  type RoomEnvironment,
  type RoomFailureReason,
  type RoomGrant,
  type RoomKind,
  type RoomMember,
  type RoomRoster,
  type RoomSession,
  type RoomSessionCapabilities,
  type RoomSessionEvents,
  type RoomSessionOptions,
  type RoomSessionSnapshot,
  type Signal,
  type SignalingPort,
  type SignalKind,
} from "./types";
