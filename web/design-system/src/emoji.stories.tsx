// SPDX-License-Identifier: AGPL-3.0-only

// The emoji of the self-hosted Noto Color Emoji (emoji.css, ADR-0091 §8–§10), one overview: a sample of each split, in
// running text of the system's sans, whose stack puts "my Noto Color Emoji" first; and the text symbols its
// unicode-range leaves to the text fonts. A page with no emoji, the other story, loads none of the splits.

import { text } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { expect } from "storybook/test";

/** A sample of each split of emoji.css, named as its file: a ZWJ sequence and a skin tone among them. */
const samples = [
  ["smileys-emotion", "😀 😂 🥹 ❤️ 🫠"],
  ["people-body", "👋 👍🏽 🧑‍💻 👩🏿‍🦰 🫶"],
  ["component", "🏻 🏿 🦰"],
  ["animals-nature", "🐱 🐶 🌸 🍀 🪼"],
  ["food-drink", "🍣 🍜 🍙 🍵 🫖"],
  ["travel-places", "🗻 🚃 ☀️ 🌈 🏯"],
  ["activities", "🎉 ⚽ 🎮 🎨 🪩"],
  ["objects", "📱 💡 📚 🔑 🔔"],
  ["symbols", "✅ ❌ ♻️ 🆗 ⚠️"],
  ["flags", "🏁 🏳️‍🌈 🏴‍☠️"],
  ["country-flags", "🇯🇵 🇺🇸 🇧🇷 🇪🇺"],
] as const;

const body: CSSProperties = { fontSize: `${text.body.size}rem`, lineHeight: text.body.lineHeight };
const caption: CSSProperties = {
  color: "var(--color-muted-text)",
  fontSize: `${text.caption.size}rem`,
  lineHeight: text.caption.lineHeight,
};

function EmojiSheet() {
  return (
    <div style={{ display: "grid", gap: "0.5rem" }}>
      <div style={caption}>Noto Color Emoji (SIL OFL 1.1): COLRv1, or OpenType-SVG in WebKit</div>
      {samples.map(([split, emoji]) => (
        <div key={split} data-split={split} style={{ display: "grid", gap: "0.125rem" }}>
          <code style={caption}>{split}</code>
          <span style={body}>{emoji}</span>
        </div>
      ))}
      <div style={{ display: "grid", gap: "0.125rem" }}>
        <code style={caption}>text symbols, drawn by the text fonts</code>
        <span style={body}>#1 * © ® ™ ‼ ↔</span>
      </div>
    </div>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Emoji", tags: ["themes"] } satisfies Meta;
export default meta;

/** The emoji face at the size of running text, as document.fonts takes a font. */
const emojiFont = `${text.body.size}rem "my Noto Color Emoji"`;

/** One sample of each split, which loads every split of the browser's format. */
export const All: StoryObj = {
  render: () => <EmojiSheet />,
  play: async ({ canvasElement }) => {
    for (const [split, emoji] of samples) {
      const sample = canvasElement.querySelector<HTMLElement>(`[data-split="${split}"] span`);
      if (sample === null) throw new Error(`no sample of the split ${split}`);
      // The stack of the text draws the emoji: the self-hosted face first (ADR-0091 §10).
      await expect(getComputedStyle(sample).fontFamily).toMatch(/^"my Noto Color Emoji",/);
      // A face of the family claims the emoji, it loads, and the family then draws them all.
      const faces = await document.fonts.load(emojiFont, emoji);
      await expect(faces.length).toBeGreaterThan(0);
      await expect(faces.map((f) => f.status)).not.toContain("error");
      await expect(document.fonts.check(emojiFont, emoji)).toBe(true);
    }
    // The keycap bases and the symbols of running text stay with the text fonts: no face claims them.
    await expect(await document.fonts.load(emojiFont, "#*0123456789©®™‼⁉↔")).toEqual([]);
  },
};

/** Running text with no emoji, which loads none of the splits. */
export const WithoutEmoji: StoryObj = {
  render: () => <p style={body}>A post with no emoji draws every character with the system fonts.</p>,
  play: async ({ canvasElement }) => {
    const paragraph = canvasElement.querySelector("p");
    if (paragraph === null) throw new Error("no paragraph");
    await expect(getComputedStyle(paragraph).fontFamily).toMatch(/^"my Noto Color Emoji",/);
    // No face of the family claims a character of the text, so none of the splits is fetched for it.
    await expect(await document.fonts.load(emojiFont, paragraph.textContent)).toEqual([]);
  },
};
