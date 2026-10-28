// SPDX-License-Identifier: AGPL-3.0-only
// The images of the demo scenario (ADR-0117 §1), drawn by code so that the repository holds no photos of third
// parties: each is a PNG (ISO/IEC 15948) of flat shapes over a vertical gradient, the same bytes on every run.
import { deflateSync } from "node:zlib";

/** An sRGB color as [red, green, blue], each 0 to 255. */
export type RGB = readonly [number, number, number];

/** A shape painted over the gradient, in pixels of the image. */
type Shape =
  | { kind: "circle"; x: number; y: number; r: number; color: RGB }
  | { kind: "rect"; x: number; y: number; w: number; h: number; color: RGB }
  /** A band whose top edge follows a sine wave, from `y` down to the bottom: hills, waves, a skyline. */
  | { kind: "wave"; y: number; amplitude: number; period: number; phase: number; color: RGB };

/** What to draw: the size, the gradient from the top color to the bottom one, and the shapes in order. */
export type Drawing = {
  width: number;
  height: number;
  top: RGB;
  bottom: RGB;
  shapes: readonly Shape[];
};

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
  const length = Buffer.alloc(4);
  length.writeUInt32BE(data.length);
  const body = Buffer.concat([Buffer.from(type, "latin1"), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(body));
  return Buffer.concat([length, body, crc]);
}

function covers(shape: Shape, x: number, y: number): boolean {
  switch (shape.kind) {
    case "circle":
      return (x - shape.x) ** 2 + (y - shape.y) ** 2 <= shape.r ** 2;
    case "rect":
      return x >= shape.x && x < shape.x + shape.w && y >= shape.y && y < shape.y + shape.h;
    case "wave":
      return y >= shape.y + shape.amplitude * Math.sin((2 * Math.PI * x) / shape.period + shape.phase);
  }
}

/** Draws `d` as the bytes of a PNG file (8-bit RGB, no interlace). */
export function drawPNG(d: Drawing): Buffer {
  const stride = d.width * 3 + 1;
  const raw = Buffer.alloc(stride * d.height);
  for (let y = 0; y < d.height; y++) {
    const t = d.height === 1 ? 0 : y / (d.height - 1);
    const mix = (a: number, b: number) => Math.round(a * (1 - t) + b * t);
    const sky: RGB = [mix(d.top[0], d.bottom[0]), mix(d.top[1], d.bottom[1]), mix(d.top[2], d.bottom[2])];
    raw[y * stride] = 0; // filter: none
    for (let x = 0; x < d.width; x++) {
      let color = sky;
      for (const shape of d.shapes) if (covers(shape, x, y)) color = shape.color;
      raw.set(color, y * stride + 1 + x * 3);
    }
  }
  const header = Buffer.alloc(13);
  header.writeUInt32BE(d.width, 0);
  header.writeUInt32BE(d.height, 4);
  header.set([8, 2, 0, 0, 0], 8); // bit depth 8, color type RGB, deflate, adaptive filtering, no interlace
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", header),
    chunk("IDAT", deflateSync(raw)),
    chunk("IEND", new Uint8Array()),
  ]);
}
