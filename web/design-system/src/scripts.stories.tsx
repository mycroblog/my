// SPDX-License-Identifier: AGPL-3.0-only

// Lines of text that tell which font draws a script (ADR-0009 §5, ADR-0091 §8–§10), each alone on its page, so that
// the page fetches the font files of that line and no other: letters of Polish and Ukrainian that Noto Sans CJK lacks,
// also inside Japanese text; Traditional Chinese alone; one emoji in a sentence; and Latin, Han and an emoji together,
// where a theme's own font takes the Latin alone. The layout tests read the font that drew each (web/e2e/catalog).

import { text } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";

const body: CSSProperties = { fontSize: `${text.body.size}rem`, lineHeight: text.body.lineHeight, margin: 0 };

const meta = { title: "Scripts" } satisfies Meta;
export default meta;

/** Polish and Ukrainian, whose ż ó ł ć ę ś ą ń ї Noto Sans CJK lacks, each in its language and inside Japanese text. */
export const LettersOutsideCjk: StoryObj = {
  render: () => (
    <div style={{ display: "grid", gap: "0.5rem" }}>
      <p lang="pl" style={body}>
        Zażółć gęślą jaźń
      </p>
      <p lang="uk" style={body}>
        їжак
      </p>
      <p lang="ja" style={body}>
        Zażółć gęślą jaźń їжак
      </p>
    </div>
  ),
};

/** A line in Traditional Chinese alone. */
export const TraditionalChinese: StoryObj = {
  render: () => (
    <p lang="zh-TW" style={body}>
      我們一起喝茶
    </p>
  ),
};

/** One emoji at the end of a sentence. */
export const OneEmoji: StoryObj = {
  render: () => <p style={body}>Bread from the morning bake 🍞</p>,
};

/** Latin, a Han character and an emoji in one line. */
export const LatinHanAndEmoji: StoryObj = {
  render: () => <p style={body}>Tea time 茶 🍵</p>,
};
