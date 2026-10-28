// SPDX-License-Identifier: AGPL-3.0-only
// A decoder of blurhash (https://blurha.sh), the few dozen characters the server gives for an image
// (MediaAttachment.blurhash) to stand in for it while it is concealed or loading. A blurhash is a grid of
// cosine (DCT) components: the first is the average color, the rest the variations around it, each
// quantized and written in base 83. Decoding sums the components for every pixel.

const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~";

/** The value of `text` in base 83, or NaN when it has a character outside the alphabet. */
function base83(text: string): number {
  let value = 0;
  for (const char of text) {
    const digit = digits.indexOf(char);
    if (digit < 0) return Number.NaN;
    value = value * 83 + digit;
  }
  return value;
}

/** An sRGB channel (0–255) in linear light (0–1). */
function toLinear(channel: number): number {
  const v = channel / 255;
  return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
}

/** A linear channel (any value, clamped to 0–1) in sRGB (0–255). */
function toSrgb(linear: number): number {
  const v = Math.min(1, Math.max(0, linear));
  return Math.round((v <= 0.0031308 ? v * 12.92 : 1.055 * v ** (1 / 2.4) - 0.055) * 255);
}

/** `base` raised to `exponent`, keeping the sign of `base`. */
function signedPow(base: number, exponent: number): number {
  return Math.sign(base) * Math.abs(base) ** exponent;
}

type Linear = readonly [number, number, number];

/** The components of `hash` in linear light, row by row, and the number of columns; null for an invalid hash. */
function components(hash: string, punch: number): { colors: Linear[]; columns: number } | null {
  if (hash.length < 6) return null;
  const size = base83(hash.charAt(0));
  const columns = (size % 9) + 1;
  const rows = Math.floor(size / 9) + 1;
  if (Number.isNaN(size) || hash.length !== 4 + 2 * columns * rows) return null;
  const maximum = (base83(hash.charAt(1)) + 1) / 166;
  const average = base83(hash.slice(2, 6));
  if (Number.isNaN(maximum) || Number.isNaN(average)) return null;
  const colors: Linear[] = [
    [toLinear(Math.floor(average / 65536)), toLinear(Math.floor(average / 256) % 256), toLinear(average % 256)],
  ];
  for (let i = 1; i < columns * rows; i++) {
    const value = base83(hash.slice(4 + i * 2, 6 + i * 2));
    if (Number.isNaN(value)) return null;
    const scale = maximum * punch;
    const channel = (quantized: number) => signedPow((quantized - 9) / 9, 2) * scale;
    colors.push([channel(Math.floor(value / 361)), channel(Math.floor(value / 19) % 19), channel(value % 19)]);
  }
  return { colors, columns };
}

/**
 * The pixels of `hash` at `width` × `height`, as RGBA bytes for an `ImageData`; null when `hash` is not a
 * blurhash. `punch` above 1 strengthens the contrast of the variations.
 */
export function decodeBlurhash(
  hash: string,
  width: number,
  height: number,
  punch = 1,
): Uint8ClampedArray<ArrayBuffer> | null {
  const decoded = components(hash, punch);
  if (decoded === null) return null;
  const { colors, columns } = decoded;
  const rows = colors.length / columns;
  const pixels = new Uint8ClampedArray(width * height * 4);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      let r = 0;
      let g = 0;
      let b = 0;
      for (let j = 0; j < rows; j++) {
        for (let i = 0; i < columns; i++) {
          const basis = Math.cos((Math.PI * x * i) / width) * Math.cos((Math.PI * y * j) / height);
          const [cr, cg, cb] = colors[i + j * columns] ?? [0, 0, 0];
          r += cr * basis;
          g += cg * basis;
          b += cb * basis;
        }
      }
      const at = 4 * (x + y * width);
      pixels[at] = toSrgb(r);
      pixels[at + 1] = toSrgb(g);
      pixels[at + 2] = toSrgb(b);
      pixels[at + 3] = 255;
    }
  }
  return pixels;
}
