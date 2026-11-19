// SPDX-License-Identifier: AGPL-3.0-only

// The browser's media devices, for the audio of a call (ADR-0101 §7): @my/rtc takes them from here, since only this
// package reads navigator.

/** The media devices (the microphone and the outputs), or undefined where the browser gives none: outside HTTPS. */
export function mediaDevices(): MediaDevices | undefined {
  // The DOM's types say it is always there; outside a secure context it is undefined.
  const devices: MediaDevices | undefined = typeof navigator === "undefined" ? undefined : navigator.mediaDevices;
  return devices;
}

/** An output of sound the browser names: its ID, and its label (empty until the site may use the microphone). */
export type AudioOutput = { readonly id: string; readonly label: string };

/** The outputs of sound there are now (the speakers, the headphones); none where the browser gives none. */
export async function audioOutputs(): Promise<readonly AudioOutput[]> {
  const devices = mediaDevices();
  if (devices === undefined) return [];
  const all = await devices.enumerateDevices().catch((): MediaDeviceInfo[] => []);
  return all.filter((d) => d.kind === "audiooutput").map((d) => ({ id: d.deviceId, label: d.label }));
}

/** Calls `onChange` when the media devices change (one plugged in or out), until the function it returns is called. */
export function watchMediaDevices(onChange: () => void): () => void {
  const devices = mediaDevices();
  devices?.addEventListener("devicechange", onChange);
  return () => {
    devices?.removeEventListener("devicechange", onChange);
  };
}
