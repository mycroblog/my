// SPDX-License-Identifier: AGPL-3.0-only

// The icons of the design system, one overview: each meaning with its glyph off and on, at the size of a timeline's
// action and of a caption, the Tabler glyph it is, and whether it flips in a right-to-left page.

import { rootPx, text } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { Icon, icons, type IconName } from "./icons";

const rem = (px: number) => `${px / rootPx}rem`;

const caption: CSSProperties = {
  color: "var(--color-muted-text)",
  fontSize: `${text.caption.size}rem`,
  lineHeight: text.caption.lineHeight,
};

type Named = { displayName?: string | undefined };
type Entry = { off: Named; on?: Named | "tile"; directional?: boolean };

/** What the glyph is: the Tabler glyph off, and on (another glyph, or the off one knocked out of a tile). */
const about = ({ off, on, directional }: Entry) =>
  [
    off.displayName,
    on === undefined ? "" : on === "tile" ? "on: tile" : `on: ${on.displayName ?? ""}`,
    directional ? "flips in RTL" : "",
  ]
    .filter(Boolean)
    .join(" · ");

function GlyphSheet() {
  const entries = Object.entries(icons) as [IconName, Entry][];
  return (
    <div style={{ display: "grid", gap: rem(10), gridTemplateColumns: "minmax(0, 1fr)" }}>
      <div style={caption}>Tabler (@tabler/icons-react 3.51.0, MIT), stroke 2</div>
      {entries.map(([name, entry]) => (
        <div
          key={name}
          style={{
            alignItems: "center",
            display: "grid",
            gap: rem(12),
            gridTemplateColumns: `${rem(112)} ${rem(64)} minmax(0, 1fr)`,
          }}
        >
          <code>{name}</code>
          <span style={{ alignItems: "center", display: "flex", gap: rem(8) }}>
            <Icon name={name} place="action" />
            {entry.on !== undefined && <Icon name={name} place="action" on />}
            <Icon name={name} place="caption" />
          </span>
          <span style={caption}>{about(entry)}</span>
        </div>
      ))}
    </div>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Glyphs", tags: ["themes"] } satisfies Meta;
export default meta;

export const All: StoryObj = { render: () => <GlyphSheet /> };
