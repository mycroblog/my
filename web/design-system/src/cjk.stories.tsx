// SPDX-License-Identifier: AGPL-3.0-only

// The text of the self-hosted Noto Sans CJK (cjk.css, ADR-0009 §5): the same Han characters in each language, which
// draws them with its region's glyphs, beside kana and Hangul; and a page in Latin alone, which loads no slice of Han.

import { text } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { ContentBody } from "./content";

/** A line in each language of the regions, with the Han characters whose glyphs differ by region: 直 骨 角 写 誤. */
const samples = [
  ["en", "Straight bone: 直 骨 角 写 誤"],
  ["ja", "直線の骨と角を写す誤り"],
  ["zh-CN", "直线的骨与角写错了"],
  ["zh-TW", "直線的骨與角寫錯了"],
  ["ko", "직선 直 骨 角 寫 誤 한국어"],
] as const;

const body: CSSProperties = { fontSize: `${text.body.size}rem`, lineHeight: text.body.lineHeight };
const caption: CSSProperties = {
  color: "var(--color-muted-text)",
  fontSize: `${text.caption.size}rem`,
  lineHeight: text.caption.lineHeight,
};

function CjkSheet() {
  return (
    <div style={{ display: "grid", gap: "0.5rem" }}>
      <div style={caption}>Noto Sans CJK (SIL OFL 1.1), by the language of the text</div>
      {samples.map(([lang, line]) => (
        <div key={lang} style={{ display: "grid", gap: "0.125rem" }}>
          <code style={caption}>{lang}</code>
          <span lang={lang} data-lang={lang} style={body}>
            {line}
          </span>
        </div>
      ))}
    </div>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "CJK", tags: ["themes"] } satisfies Meta;
export default meta;

/** The same characters in each region's language, drawn with that region's glyphs. */
export const Regions: StoryObj = { render: () => <CjkSheet /> };

/**
 * Code in a post's body, inline and as a block, with Han and an emoji, also where the code or a part of it has a
 * language: each draws with the mono stack, whose Noto Sans CJK is of the code's region.
 */
export const Code: StoryObj = {
  render: () => (
    <ContentBody lang="ja">
      <p>
        <code data-probe="inline">const 茶 = "🍵";</code> と{" "}
        <code data-probe="code-lang" lang="zh-CN">
          直线
        </code>
      </p>
      <pre data-probe="block">
        <code>
          {"// 直 "}
          <span data-probe="part-lang" lang="ko">
            한국어 直
          </span>
        </code>
      </pre>
    </ContentBody>
  ),
};

/** Running text in Latin alone, which draws with Noto Sans and loads no slice of Noto Sans CJK. */
export const LatinOnly: StoryObj = {
  render: () => (
    <p style={{ ...body, marginBlock: "1em" }}>
      A post in English draws its Latin letters with Noto Sans and loads no Han. Zażółć gęślą jaźń. Їжак.
    </p>
  ),
};
