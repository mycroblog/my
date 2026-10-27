// SPDX-License-Identifier: AGPL-3.0-only
// AvatarStack stories: the speakers stacked with one talking, more than fit, and the small size of a panel's row.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { AvatarStack } from "./AvatarStack";
import { Card } from "./Card";

const meta = {
  title: "Voice/AvatarStack",
  component: AvatarStack,
  args: {
    avatars: [
      { id: "1", name: "Haruka", speaking: true },
      { id: "2", name: "Sota" },
      { id: "3", name: "Mio" },
      { id: "4", name: "Kenji" },
      { id: "5", name: "Yui" },
    ],
  },
  // The ring between the avatars is the color of the card they sit on.
  decorators: [
    (Story) => (
      <Card padding="4">
        <Story />
      </Card>
    ),
  ],
} satisfies Meta<typeof AvatarStack>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The first three, the one talking ringed; hidden from assistive technology. */
export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByText("H")).toBeVisible();
    await expect(canvas.queryByText("K")).toBeNull();
  },
};

/** Four of them, as a room's card shows. */
export const Four: Story = { args: { max: 4 } };

/** The small avatars of a row in the third column's panel. */
export const Small: Story = { args: { size: "xs" } };
