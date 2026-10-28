// SPDX-License-Identifier: AGPL-3.0-only
// The notification sound (ADR-0109 §5): one built-in sound that the app plays while it is open, when a new notification
// arrives over the event stream and its type's sound is on. Like the autoplay of animations (lib/autoplay), it is how
// this device behaves, so the settings are kept in localStorage (lib/storage) and never sent to the server: a switch of
// the whole sound, off by default, and the types that sound. A burst of notifications plays the sound once, and
// nothing plays while the browser blocks autoplay; the settings' "Play" button plays it from a press, which lets the
// browser play it later. The sound is synthesized by `synthesizeNotificationSound` in notificationSoundAsset.test.ts,
// which checks that notification-sound.wav is its output.
import soundUrl from "./notification-sound.wav";
import { type NotificationSoundSettings, notificationSoundKey, useStorage } from "./storage";

/** How long after a sounding notification the next ones stay silent, in ms: a burst sounds once. */
export const notificationSoundQuietMs = 3000;

/** The one audio element, made on first use: a browser that allowed it once (Safari) lets it play again. */
let audio: HTMLAudioElement | undefined;
/** When the last notification of a sounding type arrived; a burst keeps it moving. */
let lastArrival = Number.NEGATIVE_INFINITY;

function element(): HTMLAudioElement {
  audio ??= new Audio(soundUrl);
  return audio;
}

/** Plays the sound from its start; the promise rejects when the browser does not allow it or cannot play it. */
async function play(): Promise<void> {
  const sound = element();
  sound.currentTime = 0;
  await sound.play();
}

/** The settings of the notification sound on this device, rendering again when they change here or in another tab. */
export function useNotificationSound(): NotificationSoundSettings {
  return useStorage(notificationSoundKey);
}

/** Turns the whole notification sound on or off on this device, keeping which types sound. */
export function setNotificationSoundEnabled(enabled: boolean): void {
  notificationSoundKey.write({ ...notificationSoundKey.read(), enabled });
}

/** Turns the sound of `types` (one row of the notification settings) on or off on this device. */
export function setNotificationSoundTypes(types: readonly string[], on: boolean): void {
  const settings = notificationSoundKey.read();
  const rest = settings.types.filter((type) => !types.includes(type));
  notificationSoundKey.write({ ...settings, types: on ? [...rest, ...types] : rest });
}

/**
 * Plays the sound for a new notification of `type` from the event stream, when the sound is on and so is the type's,
 * unless another one arrived within `notificationSoundQuietMs` (a burst plays one sound). When the browser blocks
 * autoplay, nothing plays.
 */
export function playNotificationSound(type: string): void {
  const settings = notificationSoundKey.read();
  if (!settings.enabled || !settings.types.includes(type)) return;
  const now = Date.now();
  const quiet = now - lastArrival >= notificationSoundQuietMs;
  lastArrival = now;
  if (!quiet) return;
  play().catch(() => {
    // Autoplay is blocked, or the sound cannot be played: the notification arrives without it.
  });
}

/**
 * Plays the sound once from a press of the settings' button, which lets the browser play it when notifications arrive.
 * Resolves to whether it played.
 */
export async function tryNotificationSound(): Promise<boolean> {
  try {
    await play();
    return true;
  } catch {
    return false;
  }
}

/** Forgets the audio element and the last arrival. For the test setup only. */
export function resetNotificationSound(): void {
  audio = undefined;
  lastArrival = Number.NEGATIVE_INFINITY;
}
