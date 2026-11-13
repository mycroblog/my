// SPDX-License-Identifier: AGPL-3.0-only

// The avatar of @my/design-system on the catalog: each kind of image at the preview, post and inline sizes, and the
// inline size beside a name, in one scheme. The images are plain generated ones, not photos of real people: a dusk
// landscape, a bright snowfield whose edge meets a white surface, and a dark logo on transparency.

import { Avatar, type AvatarSize } from "@my/design-system";
import { rootPx, text, type Scheme } from "@my/design-tokens";
import type { CSSProperties, ReactNode } from "react";

const rem = (px: number) => `${px / rootPx}rem`;

const svg = (body: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128" width="256" height="256">${body}</svg>`)}`;
const grain = `<filter id="g"><feTurbulence type="fractalNoise" baseFrequency="1.2" numOctaves="2" stitchTiles="stitch"/><feColorMatrix type="saturate" values="0"/></filter><rect width="128" height="128" filter="url(#g)" opacity=".12"/>`;
export const images = {
  dusk: svg(
    `<defs><linearGradient id="s" x2="0" y2="1"><stop offset="0" stop-color="#34405f"/><stop offset=".5" stop-color="#b9786a"/><stop offset=".72" stop-color="#eab486"/></linearGradient><radialGradient id="u"><stop offset="0" stop-color="#fff2d0"/><stop offset=".35" stop-color="#ffd59a" stop-opacity=".8"/><stop offset="1" stop-color="#ffd59a" stop-opacity="0"/></radialGradient></defs><rect width="128" height="128" fill="url(#s)"/><circle cx="78" cy="78" r="30" fill="url(#u)"/><path d="M0 84 L22 70 L40 78 L62 60 L86 76 L104 66 L128 74 V128 H0Z" fill="#5a4552"/><path d="M0 98 Q30 84 58 94 T128 90 V128 H0Z" fill="#3a2c35"/><path d="M0 112 Q40 102 128 110 V128 H0Z" fill="#241b22"/>${grain}`,
  ),
  snow: svg(
    `<defs><linearGradient id="s" x2="0" y2="1"><stop offset="0" stop-color="#e9edf1"/><stop offset=".6" stop-color="#f7f8f9"/><stop offset="1" stop-color="#ffffff"/></linearGradient></defs><rect width="128" height="128" fill="url(#s)"/><path d="M0 80 Q40 70 70 78 T128 74 V128 H0Z" fill="#fdfdfd"/><g fill="#3d4a44"><path d="M34 86 L42 58 L50 86Z"/><path d="M52 90 L58 66 L64 90Z"/><path d="M88 84 L97 52 L106 84Z"/></g><path d="M0 100 Q60 92 128 100 V128 H0Z" fill="#ffffff"/>${grain}`,
  ),
  logo: svg(
    `<g fill="none" stroke="#1d1d1f" stroke-width="12" stroke-linecap="round"><circle cx="50" cy="64" r="26"/><path d="M84 40 L100 88"/><path d="M100 40 L116 88" opacity=".55"/></g><circle cx="50" cy="64" r="7" fill="#1d1d1f"/>`,
  ),
} as const;

const caption: CSSProperties = {
  color: "var(--color-muted-text)",
  fontSize: `${text.caption.size}rem`,
  lineHeight: text.caption.lineHeight,
  margin: 0,
};

const sizes: AvatarSize[] = ["preview", "post", "inline"];

const samples: { label: string; name: string; src?: string }[] = [
  { label: "Photo", name: "Alice Chen", src: images.dusk },
  { label: "Bright photo", name: "佐藤 美香", src: images.snow },
  { label: "Logo, transparent", name: "Open Source Weekly", src: images.logo },
  { label: "No image", name: "Ravi" },
  { label: "No image, ja", name: "たくや" },
];

const Row = ({ label, children }: { label: string; children: ReactNode }) => (
  <div
    style={{ alignItems: "center", display: "grid", gap: rem(12), gridTemplateColumns: `${rem(96)} minmax(0, 1fr)` }}
  >
    <span style={caption}>{label}</span>
    <div style={{ alignItems: "center", display: "flex", gap: rem(16) }}>{children}</div>
  </div>
);

/** Each kind of image at the preview (64 px), post (40 px) and inline (20 px) sizes, in one scheme. */
export function AvatarSpecimen({ scheme }: { scheme: Scheme }) {
  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: "1px solid #ccc",
        borderRadius: 8,
        color: "var(--color-text)",
        colorScheme: scheme,
        display: "grid",
        flex: `1 1 ${rem(320)}`,
        gap: rem(12),
        gridTemplateColumns: "minmax(0, 1fr)",
        padding: rem(16),
      }}
    >
      <div style={caption}>{scheme} · preview / post / inline</div>
      {samples.map((s) => (
        <Row key={s.label} label={s.label}>
          {sizes.map((size) => (
            <Avatar key={size} name={s.name} src={s.src} size={size} />
          ))}
        </Row>
      ))}
      <Row label="Inline, by a name">
        <span style={{ ...caption, alignItems: "center", display: "flex", flexWrap: "wrap", gap: rem(6) }}>
          Replying to
          <Avatar name="Alice Chen" src={images.dusk} size="inline" decorative />
          <span style={{ color: "var(--color-text)" }}>Alice Chen</span>
          <Avatar name="たくや" size="inline" decorative />
          <span style={{ color: "var(--color-text)" }}>たくや</span>
        </span>
      </Row>
    </div>
  );
}
