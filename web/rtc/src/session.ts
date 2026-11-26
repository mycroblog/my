// SPDX-License-Identifier: AGPL-3.0-only
// Creates the RoomSession of a grant (ADR-0101 §7): P2P, my's SFU, LiveKit or Sora, all behind the same interface.
// LiveKit and Sora load their SDK with import() once the grant names them (ADR-0101 §15); any other kind is refused.
import { createLiveKitSession } from "./livekit";
import { reportQuality } from "./measure";
import { createP2PSession } from "./p2p";
import { createSfuSession } from "./sfu-node";
import { createSoraSession } from "./sora";
import type { RoomSession, RoomSessionOptions } from "./types";

/** The grant names a kind that this package does not connect with, or P2P comes without its signaling port. */
export class UnsupportedRoomKindError extends Error {
  override name = "UnsupportedRoomKindError";
  readonly kind: string;

  constructor(kind: string, message = `rtc: the kind ${kind} is not supported`) {
    super(message);
    this.kind = kind;
  }
}

/**
 * Joins the session `options.session` to the audio of the room that `options.grant` names; its quality goes where
 * `measureCallsWith` of measure.ts says.
 */
export function createRoomSession(options: RoomSessionOptions): RoomSession {
  return reportQuality(createSession(options));
}

/** The session of the kind the grant names. */
function createSession(options: RoomSessionOptions): RoomSession {
  switch (options.grant.kind) {
    case "p2p":
      if (options.signaling === undefined)
        throw new UnsupportedRoomKindError("p2p", "rtc: a P2P session needs its signaling port");
      return createP2PSession(options, options.signaling);
    case "sfu":
      return createSfuSession(options);
    case "livekit":
      return createLiveKitSession(options);
    case "sora":
      return createSoraSession(options);
    default:
      throw new UnsupportedRoomKindError(options.grant.kind);
  }
}
