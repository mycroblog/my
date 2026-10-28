// SPDX-License-Identifier: AGPL-3.0-only
// The source of the notification sound (lib/notificationSound): notification-sound.wav is not recorded or taken from
// elsewhere, it is the output of synthesizeNotificationSound below, made for this project and under its license. To
// change the sound, change the function and write the file again with
// `WRITE_NOTIFICATION_SOUND=1 bun x vitest run src/lib/notificationSoundAsset.test.ts` in app/web.
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

const sampleRate = 24_000;
const seconds = 0.5;

/** A soft two-note chime (A5, then E6), each note a sine with a little of its octave, a 5 ms attack and a decay. */
function synthesizeNotificationSound(): Uint8Array {
  const notes = [
    { frequency: 880, start: 0, gain: 0.32 },
    { frequency: 1318.51, start: 0.09, gain: 0.26 },
  ];
  const count = Math.round(sampleRate * seconds);
  const samples = new Int16Array(count);
  for (let i = 0; i < count; i++) {
    const t = i / sampleRate;
    let value = 0;
    for (const note of notes) {
      const local = t - note.start;
      if (local < 0) continue;
      const attack = Math.min(1, local / 0.005);
      const decay = Math.exp(-local / 0.11);
      const phase = 2 * Math.PI * note.frequency * local;
      value += note.gain * attack * decay * (Math.sin(phase) + 0.15 * Math.sin(2 * phase));
    }
    // The last 20 ms fade to silence, so that the end does not click.
    const fade = Math.min(1, (seconds - t) / 0.02);
    samples[i] = Math.round(Math.max(-1, Math.min(1, value * fade)) * 32_767);
  }
  return wav(samples);
}

/** A 16-bit mono PCM WAVE file of `samples`. */
function wav(samples: Int16Array): Uint8Array {
  const data = samples.length * 2;
  const bytes = new Uint8Array(44 + data);
  const view = new DataView(bytes.buffer);
  const text = (offset: number, value: string) => {
    for (let i = 0; i < value.length; i++) view.setUint8(offset + i, value.charCodeAt(i));
  };
  text(0, "RIFF");
  view.setUint32(4, 36 + data, true);
  text(8, "WAVE");
  text(12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true); // PCM
  view.setUint16(22, 1, true); // mono
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  text(36, "data");
  view.setUint32(40, data, true);
  samples.forEach((sample, i) => {
    view.setInt16(44 + i * 2, sample, true);
  });
  return bytes;
}

// Vitest runs in app/web (the jsdom environment gives the module an http: URL, not its file).
const file = join(process.cwd(), "src/lib/notification-sound.wav");

test("the notification sound is the output of its synthesizer", () => {
  const synthesized = synthesizeNotificationSound();
  if (process.env["WRITE_NOTIFICATION_SOUND"] === "1") writeFileSync(file, synthesized);
  expect(new Uint8Array(readFileSync(file))).toEqual(synthesized);
});
