// SPDX-License-Identifier: AGPL-3.0-only

// The name line of the post header and the relative time of @my/design-system on the catalog: the name at the start,
// the visibility glyph and the time together at the end. The specimen shows each age a post may have, from just now
// to last year, for a reader in en and one in ja, and the widest times beside long names, in one scheme.

import { Icon, RelativeTime } from "@my/design-system";
import { rootPx, text, type Scheme } from "@my/design-tokens";
import type { CSSProperties } from "react";

const rem = (px: number) => `${px / rootPx}rem`;

const second = 1000;
const minute = 60 * second;
const hour = 60 * minute;
const day = 24 * hour;

/** A moment that long ago. */
export const ago = (ms: number) => new Date(Date.now() - ms);

export type Visibility = "public" | "unlisted" | "followers" | "direct";

export const visibilityLabel: Record<Visibility, string> = {
  public: "Public",
  unlisted: "Quiet public",
  followers: "Followers only",
  direct: "Direct",
};

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

// Each age, as a moment computed when the page renders; last year is December 20 of the year before.
const lastYear = () => new Date(new Date().getFullYear() - 1, 11, 20, 18, 30);
const ages: [string, () => Date][] = [
  ["Just now", () => ago(2 * second)],
  ["45 seconds", () => ago(45 * second)],
  ["3 minutes", () => ago(3 * minute)],
  ["2 hours", () => ago(2 * hour)],
  ["Yesterday", () => ago(day + minute)],
  ["6 days", () => ago(6 * day)],
  ["3 weeks", () => ago(21 * day)],
  ["Last year", lastYear],
];

const row: CSSProperties = {
  alignItems: "baseline",
  display: "grid",
  gap: rem(12),
  gridTemplateColumns: `${rem(80)} minmax(0, 1fr)`,
};

/** Each age for a reader in en and one in ja, then long names beside the widest times, in one scheme. */
export function TimeSpecimen({ scheme }: { scheme: Scheme }) {
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
        fontSize: `${text.body.size}rem`,
        gap: rem(10),
        gridTemplateColumns: "minmax(0, 1fr)",
        padding: rem(16),
      }}
    >
      <div style={caption}>{scheme} · a reader in en, a reader in ja</div>
      {ages.map(([label, at]) => {
        const date = at();
        return (
          <div key={label} style={row}>
            <span style={caption}>{label}</span>
            <div
              style={{
                display: "grid",
                gap: `${rem(4)} ${rem(16)}`,
                gridTemplateColumns: `repeat(auto-fit, minmax(${rem(160)}, 1fr))`,
              }}
            >
              <NameLine name="Alice Chen" visibility="public" date={date} locale="en" />
              <NameLine name="佐藤 美香" visibility="public" date={date} locale="ja" />
            </div>
          </div>
        );
      })}
      <div style={row}>
        <span style={caption}>Long names</span>
        <div style={{ display: "grid", gap: rem(4), gridTemplateColumns: "minmax(0, 1fr)" }}>
          <NameLine
            name="The Open Source Weekly Newsletter Collective"
            visibility="followers"
            date={lastYear()}
            locale="en"
          />
          <NameLine
            name="株式会社みんなのマイクロブログ広報チーム公式アカウント"
            visibility="followers"
            date={lastYear()}
            locale="ja"
          />
        </div>
      </div>
    </div>
  );
}
