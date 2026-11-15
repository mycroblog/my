// SPDX-License-Identifier: AGPL-3.0-only

// A picture's blurhash (https://blurha.sh), drawn in its place while the picture itself is not loaded: under a
// sensitive cover, which loads nothing of the picture until the reader shows it. The hash is decoded here, on a small
// canvas that the box stretches; a picture without one (or with one that does not decode) leaves its box's surface.

import { useEffect, useRef } from "react";

const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~";

function base83(s: string): number {
  let value = 0;
  for (const c of s) {
    const d = digits.indexOf(c);
    if (d < 0) throw new Error("not a blurhash");
    value = value * 83 + d;
  }
  return value;
}

function toLinear(v: number): number {
  const x = v / 255;
  return x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4;
}

function toSrgb(v: number): number {
  const x = Math.min(1, Math.max(0, v));
  return Math.round(x <= 0.0031308 ? x * 12.92 * 255 : (1.055 * x ** (1 / 2.4) - 0.055) * 255);
}

const signPow = (v: number, exp: number) => Math.sign(v) * Math.abs(v) ** exp;

/** The RGBA pixels of `hash` at `width` by `height`, or undefined for a hash that does not decode. */
function decodeBlurhash(hash: string, width: number, height: number): Uint8ClampedArray | undefined {
  try {
    if (hash.length < 6) return undefined;
    const size = base83(hash.charAt(0));
    const ny = Math.floor(size / 9) + 1;
    const nx = (size % 9) + 1;
    if (hash.length !== 4 + 2 * nx * ny) return undefined;
    const max = (base83(hash.charAt(1)) + 1) / 166;
    const colors: [number, number, number][] = [];
    const dc = base83(hash.slice(2, 6));
    colors.push([toLinear(dc >> 16), toLinear((dc >> 8) & 255), toLinear(dc & 255)]);
    for (let i = 1; i < nx * ny; i++) {
      const ac = base83(hash.slice(4 + i * 2, 6 + i * 2));
      const q = (n: number) => signPow((n - 9) / 9, 2) * max;
      colors.push([q(Math.floor(ac / (19 * 19))), q(Math.floor(ac / 19) % 19), q(ac % 19)]);
    }
    const pixels = new Uint8ClampedArray(width * height * 4);
    for (let y = 0; y < height; y++)
      for (let x = 0; x < width; x++) {
        let r = 0;
        let g = 0;
        let b = 0;
        for (let j = 0; j < ny; j++)
          for (let i = 0; i < nx; i++) {
            const basis = Math.cos((Math.PI * x * i) / width) * Math.cos((Math.PI * y * j) / height);
            const c = colors[i + j * nx];
            if (c === undefined) continue;
            r += c[0] * basis;
            g += c[1] * basis;
            b += c[2] * basis;
          }
        const at = 4 * (x + y * width);
        pixels[at] = toSrgb(r);
        pixels[at + 1] = toSrgb(g);
        pixels[at + 2] = toSrgb(b);
        pixels[at + 3] = 255;
      }
    return pixels;
  } catch {
    return undefined;
  }
}

// The canvas's own size: the hash holds a handful of components, so a few pixels stretched draw it all.
const side = 32;

/** The blurhash of a picture filling its box, decorative; nothing for none. */
export function Blurhash({ hash, className }: { hash: string | undefined; className?: string | undefined }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const canvas = ref.current;
    if (canvas === null || hash === undefined) return;
    const pixels = decodeBlurhash(hash, side, side);
    const context = pixels === undefined ? null : canvas.getContext("2d");
    if (pixels === undefined || context === null) return;
    const image = context.createImageData(side, side);
    image.data.set(pixels);
    context.putImageData(image, 0, 0);
  }, [hash]);
  if (hash === undefined) return null;
  return <canvas ref={ref} className={className} width={side} height={side} aria-hidden="true" />;
}
