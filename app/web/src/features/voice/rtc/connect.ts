// SPDX-License-Identifier: AGPL-3.0-only
// Connects to the audio of a room by the method of the grant (ADR-0101 §7, ADR-0102): p2p, livekit, or sora, all behind
// the same VoiceConnection; the SFU methods load their browser SDK with import() once the grant names them (ADR-0102 §6),
// and any other method is an UnsupportedVoiceMethodError.
// The browser's environment plays each remote participant's audio through one AudioContext started from the join
// button's gesture, muted while the participant does not speak, and asks for the microphone with the constraints of
// ADR-0101 §7.
import { connectLiveKit } from "./livekit";
import { connectP2P, microphoneConstraints } from "./p2p";
import { connectSora } from "./sora";
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
    case "livekit":
      return connectLiveKit(options);
    case "sora":
      return connectSora(options);
    default:
      throw new UnsupportedVoiceMethodError(options.grant.kind);
  }
}

/** The one AudioContext of the page's calls, made and resumed from the user's gesture (the join button). */
let sharedContext: AudioContext | null = null;

/**
 * Starts the audio of calls from the user's gesture (ADR-0101 §7): browsers let audio play only after one, and
 * Safari only what the gesture itself started. The AudioContext made here plays every remote participant.
 */
function audioContext(): AudioContext {
  sharedContext ??= new AudioContext();
  if (sharedContext.state === "suspended") void sharedContext.resume().catch(() => undefined);
  return sharedContext;
}

/** The error of a context that did not start: the browser blocks the audio until a gesture resumes it. */
function blocked(context: AudioContext): DOMException | null {
  return context.state === "running" ? null : new DOMException("voice: the audio is blocked until a gesture", "NotAllowedError");
}

/**
 * The audio of one participant through the shared AudioContext, with a gain that mutes it while the participant does
 * not speak. The stream is also the source of a muted audio element, which Chrome needs for the audio of a remote
 * WebRTC stream to reach Web Audio; a muted element plays without a gesture.
 */
function contextAudio(context: AudioContext): RemoteAudio {
  const element = new Audio();
  element.muted = true;
  const gain = context.createGain();
  gain.gain.value = 0;
  gain.connect(context.destination);
  let source: MediaStreamAudioSourceNode | null = null;
  return {
    play: (stream) => {
      element.srcObject = stream;
      void element.play().catch(() => undefined);
      source?.disconnect();
      source = context.createMediaStreamSource(stream);
      source.connect(gain);
      const error = blocked(context);
      return error === null ? Promise.resolve() : Promise.reject(error);
    },
    resume: async () => {
      await context.resume();
      const error = blocked(context);
      if (error !== null) throw error;
    },
    setAudible: (audible) => {
      gain.gain.value = audible ? 1 : 0;
    },
    close: () => {
      source?.disconnect();
      gain.disconnect();
      element.pause();
      element.srcObject = null;
    },
  };
}

/** Whether the browser has what a call needs: WebRTC, Web Audio, and the microphone (a secure context). */
export function voiceSupported(): boolean {
  return typeof RTCPeerConnection === "function" && typeof AudioContext === "function" && "mediaDevices" in navigator;
}

/**
 * The browser's WebRTC, microphone, and audio. Call it from the user's gesture (the join button): it starts the
 * audio of the call then.
 */
export function browserEnvironment(): VoiceEnvironment {
  const context = audioContext();
  return {
    createPeerConnection: (configuration) => new RTCPeerConnection(configuration),
    getMicrophone: () => navigator.mediaDevices.getUserMedia(microphoneConstraints),
    createRemoteAudio: () => contextAudio(context),
  };
}
