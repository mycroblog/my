// SPDX-License-Identifier: AGPL-3.0-only

// A drawing of the scenario (drawing.ts) as the bytes of a PNG file (ISO/IEC 15948: 8-bit RGB, no interlace), the
// same bytes on every run, for the seed to upload.

import { deflateSync } from "node:zlib";
import type { Drawing, Shape } from "../drawing";

const crcTable = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});

function crc32(bytes: Uint8Array): number {
  let c = 0xffffffff;
  for (const b of bytes) c = (crcTable[(c ^ b) & 0xff] ?? 0) ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

function chunk(type: string, data: Uint8Array): Buffer {
  const body = Buffer.concat([Buffer.from(type, "latin1"), data]);
  const out = Buffer.alloc(body.length + 8);
  out.writeUInt32BE(data.length, 0);
  body.copy(out, 4);
  out.writeUInt32BE(crc32(body), body.length + 4);
  return out;
}

function covers(s: Shape, x: number, y: number): boolean {
  switch (s.kind) {
    case "circle":
      return (x - s.x) ** 2 + (y - s.y) ** 2 <= s.r ** 2;
    case "rect":
      return x >= s.x && x < s.x + s.w && y >= s.y && y < s.y + s.h;
    case "wave":
      return y >= s.y + s.amplitude * Math.sin((2 * Math.PI * x) / s.period + s.phase);
  }
}

/** The PNG of `d`. */
export function drawPNG(d: Drawing): Uint8Array {
  const stride = d.width * 3 + 1;
  const raw = Buffer.alloc(stride * d.height);
  for (let y = 0; y < d.height; y++) {
    const t = d.height === 1 ? 0 : y / (d.height - 1);
    const mix = (i: 0 | 1 | 2) => Math.round(d.top[i] * (1 - t) + d.bottom[i] * t);
    const sky = [mix(0), mix(1), mix(2)] as const;
    raw[y * stride] = 0;
    for (let x = 0; x < d.width; x++) {
      let color: readonly number[] = sky;
      for (const s of d.shapes) if (covers(s, x, y)) color = s.color;
      raw.set(color, y * stride + 1 + x * 3);
    }
  }
  const header = Buffer.alloc(13);
  header.writeUInt32BE(d.width, 0);
  header.writeUInt32BE(d.height, 4);
  header.set([8, 2, 0, 0, 0], 8);
  return new Uint8Array(
    Buffer.concat([
      Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
      chunk("IHDR", header),
      chunk("IDAT", deflateSync(raw)),
      chunk("IEND", new Uint8Array()),
    ]),
  );
}
