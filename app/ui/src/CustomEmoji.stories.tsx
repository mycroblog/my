// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { type CustomEmojiImage, EmojiText } from "./CustomEmoji";
import { PostContent, serverSanitizedHtml } from "./PostContent";

// Plain squares stand in for emoji images: an orange still image and a blue "animated" one.
const orange =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGN4ViH3nxLMMGrAqAGjBgwXAwAvJnsfj06SzAAAAABJRU5ErkJggg==";
const blue =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGPQ6LnznxLMMGrAqAGjBgwXAwBfb48fVw/D1AAAAABJRU5ErkJggg==";

const emojis: CustomEmojiImage[] = [
  { shortcode: "blobcat", url: orange, static_url: orange },
  { shortcode: "party", url: blue, static_url: orange },
];

const meta = {
  title: "Timeline/CustomEmoji",
  component: EmojiText,
  args: { text: "Alice :blobcat: :party:", emojis },
  render: (args) => (
    <p dir="auto" className="text-fg">
      <EmojiText {...args} />
    </p>
  ),
} satisfies Meta<typeof EmojiText>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A display name with custom emoji: each is an image named by its `:shortcode:`. */
export const InPlainText: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("img", { name: ":blobcat:" })).toBeVisible();
    await expect(canvas.getByRole("img", { name: ":party:" })).toBeVisible();
  },
};

/** A shortcode that is not among the emoji stays text. */
export const UnknownShortcode: Story = {
  args: { text: "No such emoji :unknown: here" },
  play: async ({ canvas }) => {
    await expect(canvas.getByText("No such emoji :unknown: here")).toBeVisible();
  },
};

/** The animated image is left out when the user turns animations off; the still one stays. */
export const StillOnly: Story = {
  args: { allowMotion: false },
};

/** Emoji in the sanitized body of a post, drawn into its text with the DOM API. */
export const InPostBody: Story = {
  render: () => (
    <PostContent
      html={serverSanitizedHtml("<p>Hello :blobcat: world :party:</p><p><code>:blobcat:</code> stays code.</p>")}
      emojis={emojis}
      lang="en"
    />
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getAllByRole("img", { name: ":blobcat:" })).toHaveLength(1);
    await expect(canvas.getByText(":blobcat:")).toBeVisible();
  },
};
