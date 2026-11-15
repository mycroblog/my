// SPDX-License-Identifier: AGPL-3.0-only

// The images of the scenario (ADR-0015 §5), drawn by code so that the repository holds no photo of a third party:
// flat shapes over a vertical gradient. The seed uploads a drawing as a PNG (seed/png.ts); the fake API shows the same
// drawing as an SVG, which a browser draws without any file.

/** An sRGB color as [red, green, blue], each 0 to 255. */
export type RGB = readonly [number, number, number];

/** A shape painted over the gradient, in pixels of the image. */
export type Shape =
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

const hex = (c: RGB) => `#${c.map((v) => v.toString(16).padStart(2, "0")).join("")}`;

/** The top edge of a wave as an SVG path, sampled every 4 pixels, closed along the bottom. */
function wavePath(d: Drawing, s: Extract<Shape, { kind: "wave" }>): string {
  const points: string[] = [];
  for (let x = 0; x <= d.width; x += 4) {
    const y = s.y + s.amplitude * Math.sin((2 * Math.PI * x) / s.period + s.phase);
    points.push(`${x} ${y.toFixed(1)}`);
  }
  return `M${points.join(" L")} V${d.height} H0Z`;
}

/** The drawing as an SVG document. */
export function drawSVG(d: Drawing): string {
  const shapes = d.shapes.map((s) => {
    switch (s.kind) {
      case "circle":
        return `<circle cx="${s.x}" cy="${s.y}" r="${s.r}" fill="${hex(s.color)}"/>`;
      case "rect":
        return `<rect x="${s.x}" y="${s.y}" width="${s.w}" height="${s.h}" fill="${hex(s.color)}"/>`;
      case "wave":
        return `<path d="${wavePath(d, s)}" fill="${hex(s.color)}"/>`;
    }
  });
  return [
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${d.width} ${d.height}" width="${d.width}" height="${d.height}">`,
    `<defs><linearGradient id="g" x2="0" y2="1"><stop offset="0" stop-color="${hex(d.top)}"/><stop offset="1" stop-color="${hex(d.bottom)}"/></linearGradient></defs>`,
    `<rect width="${d.width}" height="${d.height}" fill="url(#g)"/>`,
    ...shapes,
    "</svg>",
  ].join("");
}

/** The drawing as a `data:` URL of its SVG, for an `<img>`. */
export const drawingURL = (d: Drawing) => `data:image/svg+xml,${encodeURIComponent(drawSVG(d))}`;

const base83 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~";
const digits83 = (v: number, n: number) =>
  Array.from({ length: n }, (_, i) => base83.charAt(Math.floor(v / 83 ** (n - 1 - i)) % 83)).join("");
const linear = (v: number) => {
  const x = v / 255;
  return x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4;
};
const srgb = (v: number) => {
  const x = Math.min(1, Math.max(0, v));
  return Math.round(x <= 0.0031308 ? x * 12.92 * 255 : (1.055 * x ** (1 / 2.4) - 0.055) * 255);
};

/**
 * The drawing's blurhash (https://blurha.sh), as the server gives an attachment's: three bands from top to bottom,
 * the gradient's top, its bottom, and the color of the front shape, in one column of components.
 */
export function drawingBlurhash(d: Drawing): string {
  const bands = [d.top, d.bottom, d.shapes.at(-1)?.color ?? d.bottom].map((c) => c.map(linear));
  const components = [0, 1, 2].map((j) =>
    [0, 1, 2].map((k) =>
      bands.reduce(
        (sum, band, y) => sum + ((j === 0 ? 1 : 2) * Math.cos((Math.PI * j * (y + 0.5)) / 3) * (band[k] ?? 0)) / 3,
        0,
      ),
    ),
  );
  const [dc = [0, 0, 0], ...ac] = components;
  const most = Math.max(...ac.flat().map(Math.abs));
  const quantised = Math.max(0, Math.min(82, Math.floor(most * 166 - 0.5)));
  const max = (quantised + 1) / 166;
  const q = (v: number) => Math.max(0, Math.min(18, Math.floor(Math.sign(v) * Math.abs(v / max) ** 0.5 * 9 + 9.5)));
  const [r = 0, g = 0, b = 0] = dc.map(srgb);
  return (
    digits83(2 * 9, 1) +
    digits83(quantised, 1) +
    digits83((r << 16) + (g << 8) + b, 4) +
    ac.map(([x = 0, y = 0, z = 0]) => digits83(q(x) * 19 * 19 + q(y) * 19 + q(z), 2)).join("")
  );
}
