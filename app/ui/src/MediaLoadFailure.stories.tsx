// SPDX-License-Identifier: AGPL-3.0-only
// MediaLoadFailure stories: the mark of media that could not load, over the place of a picture.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn } from "storybook/test";
import { Box } from "./Box";
import { MediaLoadFailure } from "./MediaLoadFailure";

const onRetry = fn();

const meta = {
  title: "Display/MediaLoadFailure",
  component: MediaLoadFailure,
  args: {
    labels: { loading: "Loading media", failed: "Couldn't load this media.", retry: "Try again" },
    onRetry,
  },
  decorators: [
    (Story) => (
      <Box width="sm">
        <div className="relative aspect-video overflow-hidden rounded-lg">
          <Story />
        </div>
      </Box>
    ),
  ],
} satisfies Meta<typeof MediaLoadFailure>;

export default meta;
type Story = StoryObj<typeof meta>;

/** What failed, over the media's place, and "Try again". */
export const Default: Story = {
  play: async ({ canvas, userEvent }) => {
    onRetry.mockClear();
    await expect(canvas.getByRole("group", { name: "Couldn't load this media." })).toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: "Try again" }));
    await expect(onRetry).toHaveBeenCalledOnce();
  },
};
