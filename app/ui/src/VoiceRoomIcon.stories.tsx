// SPDX-License-Identifier: AGPL-3.0-only
// VoiceRoomIcon stories: the emoji before a room's name, a Unicode emoji and a custom emoji, in a heading.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Heading } from "./Heading";
import { VoiceRoomIcon } from "./VoiceRoomIcon";

// A plain orange square stands in for the image of a custom emoji.
const orange =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGN4ViH3nxLMMGrAqAGjBgwXAwAvJnsfj06SzAAAAABJRU5ErkJggg==";

const meta = {
  title: "Voice/VoiceRoomIcon",
  component: VoiceRoomIcon,
  args: { emoji: { unicode: "\u{1F3B5}" } },
  decorators: [
    (Story) => (
      <Heading level={1}>
        <Story />
        <span dir="auto">Friday night radio</span>
      </Heading>
    ),
  ],
} satisfies Meta<typeof VoiceRoomIcon>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A Unicode emoji: the heading is still named by the room's name alone. */
export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("heading", { name: "Friday night radio" })).toBeVisible();
  },
};

/** A custom emoji of the server, drawn as its image at the height of the text. */
export const Custom: Story = {
  args: { emoji: { custom: { shortcode: "blobcat", url: orange, static_url: orange } } },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("heading", { name: "Friday night radio" })).toBeVisible();
  },
};
