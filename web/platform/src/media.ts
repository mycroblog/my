// SPDX-License-Identifier: AGPL-3.0-only

// The browser's media devices, for the audio of a call (ADR-0101 §7): @my/rtc takes them from here, since only this
// package reads navigator.

/** The media devices (the microphone and the outputs), or undefined where the browser gives none: outside HTTPS. */
export function mediaDevices(): MediaDevices | undefined {
  // The DOM's types say it is always there; outside a secure context it is undefined.
  const devices: MediaDevices | undefined = typeof navigator === "undefined" ? undefined : navigator.mediaDevices;
  return devices;
}
