// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Cluster } from "./Cluster";
import { EmojiTile } from "./EmojiTile";
import { Heading } from "./Heading";

const pixel =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 8 8'%3E%3Crect width='8' height='8' fill='%23e11d48'/%3E%3C/svg%3E";

const meta = {
  title: "Display/EmojiTile",
  component: EmojiTile,
  args: { emoji: { shortcode: "party", url: pixel, static_url: pixel } },
} satisfies Meta<typeof EmojiTile>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: (args) => (
    <Cluster wrap={false} align="center" gap="3">
      <EmojiTile {...args} />
      <Heading level={3} mono>
        :party:
      </Heading>
    </Cluster>
  ),
};

/** While the image is unknown the tile stays empty. */
export const Empty: Story = { args: { emoji: undefined } };
