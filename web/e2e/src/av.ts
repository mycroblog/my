// SPDX-License-Identifier: AGPL-3.0-only

// The video and the sound that an E2E test attaches to its own post, so that the demo server, which takes only real
// video and sound files, holds a post with each (fresh.ts). Both are a few kilobytes and need no tool to make: the
// sound is written here, frame by frame; the video is a fixed file of a few frames, as text.

/**
 * A silent MP3 of about `seconds` seconds: MPEG-1 Layer III frames, mono at 32 kHz and 32 kbit/s, each 144 bytes of a
 * header and an all-zero side information and body, which a decoder reads as silence.
 */
export function silentMP3(seconds: number): Uint8Array<ArrayBuffer> {
  const frameBytes = 144;
  const frameSeconds = 1152 / 32_000;
  const frames = Math.ceil(seconds / frameSeconds);
  const out = new Uint8Array(frames * frameBytes);
  for (let i = 0; i < frames; i++) {
    // Sync, MPEG-1, Layer III, no CRC; 32 kbit/s, 32 kHz, no padding; mono.
    out.set([0xff, 0xfb, 0x18, 0xc0], i * frameBytes);
  }
  return out;
}

/**
 * A WebM of two seconds of a plain blue-grey picture, 64 × 36 at five frames a second, in VP8 without sound (727
 * bytes), made by `ffmpeg -f lavfi -i color=c=0x3f6f86:s=64x36:r=5:d=2 -c:v libvpx -b:v 8k -g 10 -an -fflags
 * +bitexact -flags +bitexact -map_metadata -1 tiny.webm`.
 */
const webm =
  "GkXfo59ChoEBQveBAULygQRC84EIQoKEd2VibUKHgQJChYECGFOAZwEAAAAAAAKnEU2bdLpNu4tTq4QVSalmU6yBoU27i1OrhBZUrmtTrIHGTbuM" +
  "U6uEElTDZ1OsggENTbuMU6uEHFO7a1OsggKR7AEAAAAAAABZAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAVSalmoCrXsYMPQkBNgIRMYXZmV0GETGF2ZkSJiECfQAAAAAAAFlSua8Ku" +
  "AQAAAAAAADnXgQFzxYgAAAAAAAAAAZyBACK1nIN1bmSGhVZfVlA4g4EBI+ODhAvrwgDgAQAAAAAAAAawgUC6gSQSVMNn33NzAQAAAAAAAFVjwItj" +
  "xYgAAAAAAAAAAWfIAQAAAAAAABhFo4dFTkNPREVSRIeLTGF2YyBsaWJ2cHhnyKJFo4hEVVJBVElPTkSHlDAwOjAwOjAyLjAwMDAwMDAwMAAAH0O2" +
  "dUEa54EAo76BAACAUAMAnQEqQAAkAABHCIWFiIWEiAICAnWqA/gCBwjVZ5mPN9IA/vjQC/84ljqd3dn/9ej+ej+ej/OAAKOWgQDIANEBAAEQEAAY" +
  "ABhYL/QACIwAAKOWgQGQANEBAAEQEAAYABhYL/QACIwAAKOWgQJYANEBAAEQEAAYABhYL/QACIwAAKOWgQMgANEBAAEQEAAYABhYL/QACIwAAKOW" +
  "gQPoANEBAAEQEAAYABhYL/QACIwAAKOWgQSwANEBAAEQEAAYABhYL/QACIwAAKOVgQV4ALEBAAEQEBRgAGFgv9AAIjAAo5aBBkAA0QEAARAQABgA" +
  "GFgv9AAIjAAAo5aBBwgA0QEAARAQABgAGFgv9AAIjAAAHFO7a5G7j7OBALeK94EB8YIBcfCBAw==";

/** The two-second WebM video. */
export const tinyWebM = (): Uint8Array<ArrayBuffer> => Uint8Array.from(Buffer.from(webm, "base64"));
