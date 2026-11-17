// SPDX-License-Identifier: AGPL-3.0-only

// The pictures of the media stories: landscapes of generated shapes, as data URLs.

const svg = (w: number, h: number, body: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="${w * 4}" height="${h * 4}">${body}</svg>`)}`;

/** A landscape of generated shapes, from `top` to `low`, with ridges in `ridge`. */
const scene = (top: string, low: string, ridge: string) =>
  svg(
    160,
    100,
    `<defs><linearGradient id="s" x2="0" y2="1"><stop offset="0" stop-color="${top}"/><stop offset="1" stop-color="${low}"/></linearGradient></defs><rect width="160" height="100" fill="url(#s)"/><circle cx="112" cy="44" r="14" fill="#fff6e0"/><path d="M0 70 L40 52 L80 66 L120 48 L160 62 V100 H0Z" fill="${ridge}"/>`,
  );

export const pictures = {
  dawn: scene("#f6c7a1", "#fbe6cf", "#9a6a63"),
  sea: scene("#7fb3d5", "#d6ecf5", "#3f6f86"),
  forest: scene("#a9d0b0", "#e8f2df", "#4c7650"),
  night: scene("#1d2440", "#4b4f7a", "#2a2b45"),
};
