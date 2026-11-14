// SPDX-License-Identifier: AGPL-3.0-only

// What the stories of every layer share (ADR-0165 §1): generated avatar images, not photos of real people (a dusk
// landscape, a bright snowfield whose edge meets a white surface, and a dark logo on transparency), and the name line
// of a post header as the design system's parts draw it: the name at the start, the visibility glyph and the time
// together at the end. Only the subpath @my/design-system/samples exports it; screens never use it.

import type { CSSProperties } from "react";
import { Icon } from "./icons";
import { RelativeTime } from "./time";
import { rootPx, text } from "@my/design-tokens";

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

/** A moment that long ago. */
export const ago = (ms: number) => new Date(Date.now() - ms);

export type Visibility = "public" | "unlisted" | "followers" | "direct";

export const visibilityLabel: Record<Visibility, string> = {
  public: "Public",
  unlisted: "Quiet public",
  followers: "Followers only",
  direct: "Direct",
};

/** A story's own caption, in muted text. */
export const caption: CSSProperties = {
  color: "var(--color-muted-text)",
  fontSize: `${text.caption.size}rem`,
  lineHeight: text.caption.lineHeight,
};

const ellipsis: CSSProperties = { minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" };

/** The visibility glyph and the time, kept together; the glyph alone carries the visibility, so a wrapper names it. */
export const Stamp = ({
  visibility,
  date,
  locale,
}: {
  visibility: Visibility;
  date: Date;
  locale?: string | undefined;
}) => (
  <span style={{ ...caption, alignItems: "center", display: "flex", flex: "none", gap: rem(6) }}>
    <span role="img" aria-label={visibilityLabel[visibility]} style={{ display: "inline-flex", flex: "none" }}>
      <Icon name={visibility} place="caption" />
    </span>
    <RelativeTime date={date} locale={locale} />
  </span>
);

/**
 * The name line of the post header: the name at the start, then the stamp at the end. The name truncates before the
 * stamp gives up any room. The time is in the reader's locale, the page's unless `locale` stands in for another reader.
 */
export const NameLine = ({
  name,
  visibility,
  date,
  locale,
}: {
  name: string;
  visibility: Visibility;
  date: Date;
  locale?: string | undefined;
}) => (
  <div style={{ alignItems: "baseline", display: "flex", gap: rem(8), lineHeight: 1.3, minWidth: 0 }}>
    <span style={{ ...ellipsis, flex: "0 1 auto", fontWeight: 700 }}>{name}</span>
    <span style={{ display: "flex", flex: "none", marginInlineStart: "auto" }}>
      <Stamp visibility={visibility} date={date} locale={locale} />
    </span>
  </div>
);
