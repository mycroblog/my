// SPDX-License-Identifier: AGPL-3.0-only

// The disclosure of @my/design-system, as a post's content warning: folded, shown, its button hovered and focused
// from the keyboard at rest, a long warning, and one with no glyph, in one scheme and the reader's language.

import { Disclosure } from "@my/design-system";
import { Preview, type PreviewState } from "@my/design-system/preview";
import { rootPx, text, type Scheme } from "@my/design-tokens";
import type { CSSProperties } from "react";
import type { Lang } from "./Posts";
import { caption } from "./Times";

const rem = (px: number) => `${px / rootPx}rem`;

const sample = {
  en: {
    warning: "Spoilers for the season finale",
    long: "Spoilers for the season finale, and a long talk about the ending that runs past one line on a phone",
    body: "The ending was not what anyone expected, and the last scene changes how the first episode reads.",
  },
  ja: {
    warning: "最終回のネタバレ",
    long: "最終回のネタバレと、結末についての長い話を含みます。モバイルでは 1 行に収まりません",
    body: "誰も予想しなかった結末で、最後の場面を見ると 1 話の見え方が変わります。",
  },
} as const;

type Row = { title: string; state?: PreviewState; open?: boolean; long?: boolean; plain?: boolean };
const rows: Row[] = [
  { title: "folded" },
  { title: "shown", open: true },
  { title: "folded · button hovered", state: "hover" },
  { title: "folded · button focused from the keyboard", state: "focus-visible" },
  { title: "a long warning", long: true },
  { title: "no glyph", plain: true },
];

const panel: CSSProperties = {
  alignContent: "start",
  background: "var(--color-surface)",
  border: "1px solid #ccc",
  borderRadius: 8,
  color: "var(--color-text)",
  display: "grid",
  flex: `1 1 ${rem(320)}`,
  gap: rem(16),
  gridTemplateColumns: "minmax(0, 1fr)",
  padding: rem(16),
};

export function DisclosureSpecimen({ scheme, lang }: { scheme: Scheme; lang: Lang }) {
  const s = sample[lang];
  return (
    <div lang={lang} style={{ ...panel, colorScheme: scheme }}>
      {rows.map((row) => (
        <div key={row.title} style={{ display: "grid", gap: rem(6), gridTemplateColumns: "minmax(0, 1fr)" }}>
          <div style={caption}>
            {scheme} · {lang} · {row.title}
          </div>
          <Preview state={row.state}>
            <Disclosure
              summary={row.long ? s.long : s.warning}
              icon={row.plain ? undefined : "hidden"}
              defaultExpanded={row.open}
              locale={lang}
            >
              <p style={{ fontSize: `${text.body.size}rem`, lineHeight: text.body.lineHeight, margin: 0 }}>{s.body}</p>
            </Disclosure>
          </Preview>
        </div>
      ))}
    </div>
  );
}
