// SPDX-License-Identifier: AGPL-3.0-only
// The browser's environment of a RoomSession: WebRTC, the microphone with the constraints of ADR-0101 §7, and the
// audio of the remote participants through one AudioContext of the session, made and resumed from the user's gesture
// (the join button): browsers let audio play only after one, and Safari only what the gesture itself started. Each
// remote stream is also the source of a muted, detached audio element, which Chrome needs for the audio of a remote
// WebRTC stream to reach Web Audio; nothing is put in the page, so the session depends on no screen. The level of
// each stream, for who is talking, comes from an AnalyserNode on the same context. The output device is the
// context's (setSinkId), where the browser can choose it. Closing the environment closes the context.
//
// Only @my/platform reads navigator (ADR-0028 §17): the caller gives the media devices and the network's changes.
import type { LevelMeter, RemoteAudio, RoomEnvironment } from "./types";

/** The microphone's constraints (ADR-0101 §7): audio only. */
export const microphoneConstraints: MediaStreamConstraints = {
  audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
  video: false,
};

/** What the browser's environment takes from @my/platform. */
export type BrowserMedia = {
  /** The browser's media devices (`navigator.mediaDevices`, in a secure context). */
  readonly devices: MediaDevices;
  /** Calls `onChange` when the network comes back or changes, until the function it returns is called. */
  readonly watchNetwork?: ((onChange: () => void) => () => void) | undefined;
};

/** The error of a context that did not start: the browser blocks the audio until a gesture resumes it. */
function blocked(context: AudioContext): DOMException | null {
  return context.state === "running"
    ? null
    : new DOMException("rtc: the audio is blocked until a gesture", "NotAllowedError");
}

/** The audio of one participant through the session's AudioContext, with a gain that mutes it while it is not heard. */
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

/** The samples of a meter: about 10 ms at 48 kHz. */
const meterSamples = 512;

/** The level of `stream` through an AnalyserNode of the context, connected to nothing else (it plays nothing). */
function contextMeter(context: AudioContext, stream: MediaStream): LevelMeter | null {
  let source: MediaStreamAudioSourceNode;
  try {
    source = context.createMediaStreamSource(stream);
  } catch {
    // A stream without an audio track.
    return null;
  }
  const analyser = context.createAnalyser();
  analyser.fftSize = meterSamples;
  source.connect(analyser);
  const samples = new Float32Array(analyser.fftSize);
  return {
    level: () => {
      analyser.getFloatTimeDomainData(samples);
      let sum = 0;
      for (const sample of samples) sum += sample * sample;
      return Math.sqrt(sum / samples.length);
    },
    close: () => {
      source.disconnect();
      analyser.disconnect();
    },
  };
}

/** An AudioContext that may choose its output (Chrome and Edge 110 and later). */
type SinkContext = AudioContext & { setSinkId?: (sinkId: string) => Promise<void> };

/**
 * The browser's environment of one session. Call it from the user's gesture (the join button): it starts the audio of
 * the call then. The session it is given to closes it.
 */
export function browserEnvironment(media: BrowserMedia): RoomEnvironment {
  const context: SinkContext = new AudioContext();
  if (context.state === "suspended") void context.resume().catch(() => undefined);
  const { devices } = media;
  const setSinkId = context.setSinkId;
  return {
    createPeerConnection: (configuration) => new RTCPeerConnection(configuration),
    getMicrophone: () => devices.getUserMedia(microphoneConstraints),
    createStream: (tracks) => new MediaStream([...tracks]),
    createRemoteAudio: () => contextAudio(context),
    createLevelMeter: (stream) => contextMeter(context, stream),
    setOutputDevice:
      typeof setSinkId === "function"
        ? async (deviceId) => {
            await setSinkId.call(context, deviceId);
          }
        : undefined,
    audioInputs: async () =>
      (await devices.enumerateDevices()).filter((device) => device.kind === "audioinput").map((d) => d.deviceId),
    watchDevices: (onChange) => {
      devices.addEventListener("devicechange", onChange);
      return () => {
        devices.removeEventListener("devicechange", onChange);
      };
    },
    watchNetwork: media.watchNetwork,
    close: () => {
      void context.close().catch(() => undefined);
    },
  };
}
