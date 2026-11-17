// SPDX-License-Identifier: AGPL-3.0-only

// The pictures of the media viewer's stories: a wide one, a tall one and a night sky, as data URLs.

const svg = (w: number, h: number, body: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="${w * 8}" height="${h * 8}">${body}</svg>`)}`;

export const wide = svg(
  160,
  100,
  `<defs><linearGradient id="s" x2="0" y2="1"><stop offset="0" stop-color="#7fb3d5"/><stop offset="1" stop-color="#d6ecf5"/></linearGradient></defs><rect width="160" height="100" fill="url(#s)"/><circle cx="112" cy="40" r="12" fill="#fffbe8"/><path d="M0 64 L36 50 L70 60 L104 44 L160 58 V100 H0Z" fill="#3f6f86"/><path d="M0 82 Q60 74 160 80 V100 H0Z" fill="#2a4d5f"/>`,
);
export const tall = svg(
  90,
  160,
  `<defs><linearGradient id="s" x2="0" y2="1"><stop offset="0" stop-color="#f2b56b"/><stop offset="1" stop-color="#f9dfb0"/></linearGradient></defs><rect width="90" height="160" fill="url(#s)"/><rect x="38" y="40" width="14" height="120" fill="#5e2d22"/><circle cx="45" cy="40" r="30" fill="#c2683f"/><circle cx="30" cy="56" r="18" fill="#94462f"/>`,
);
export const night = svg(
  160,
  100,
  `<rect width="160" height="100" fill="#1d2440"/><circle cx="40" cy="30" r="10" fill="#f3efd6"/><path d="M0 70 Q50 56 100 68 T160 64 V100 H0Z" fill="#2a2b45"/>`,
);
