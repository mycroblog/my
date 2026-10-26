// SPDX-License-Identifier: AGPL-3.0-only
// Connects to the audio of a room by the method of the grant (ADR-0101 §7, ADR-0102): p2p here. The SFU methods
// (livekit, sora) come behind the same VoiceConnection; until then their grant is an UnsupportedVoiceMethodError.
// The browser's environment plays each remote participant's audio in an audio element of its own, muted while the
// participant does not speak, and asks for the microphone with the constraints of ADR-0101 §7.
import { connectP2P, microphoneConstraints } from "./p2p";
import type { RemoteAudio, VoiceConnection, VoiceConnectionOptions, VoiceEnvironment } from "./types";

/** The grant names a method that this client does not connect with. */
export class UnsupportedVoiceMethodError extends Error {
  override name = "UnsupportedVoiceMethodError";
  readonly kind: string;

  constructor(kind: string) {
    super(`voice: the method ${kind} is not supported`);
    this.kind = kind;
  }
}

/** Connects the user's session to the audio of a room by the method of `options.grant`. */
export function connectVoice(options: VoiceConnectionOptions): VoiceConnection {
  switch (options.grant.kind) {
    case "p2p":
      return connectP2P(options);
    default:
      throw new UnsupportedVoiceMethodError(options.grant.kind);
  }
}

/** The audio of one participant in an audio element that is not in the document. */
function elementAudio(): RemoteAudio {
  const element = new Audio();
  element.autoplay = true;
  return {
    play: async (stream) => {
      element.srcObject = stream;
      await element.play();
    },
    resume: () => element.play(),
    setAudible: (audible) => {
      element.muted = !audible;
    },
    close: () => {
      element.pause();
      element.srcObject = null;
    },
  };
}

/** Whether the browser has what a call needs: WebRTC and the microphone (a secure context). */
export function voiceSupported(): boolean {
  return typeof RTCPeerConnection === "function" && "mediaDevices" in navigator;
}

/** The browser's WebRTC, microphone, and audio elements. */
export function browserEnvironment(): VoiceEnvironment {
  return {
    createPeerConnection: (configuration) => new RTCPeerConnection(configuration),
    getMicrophone: () => navigator.mediaDevices.getUserMedia(microphoneConstraints),
    createRemoteAudio: elementAudio,
  };
}
