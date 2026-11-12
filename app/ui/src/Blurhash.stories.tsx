// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { Blurhash } from "./Blurhash";

const meta = {
  title: "Display/Blurhash",
  component: Blurhash,
  args: { hash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj" },
  // The blur fills its container: here a 16:9 cell on the sunken surface, as in MediaGrid.
  render: (args) => (
    <div className="aspect-video w-80 max-w-full overflow-hidden rounded-lg bg-surface-sunken">
      <Blurhash {...args} />
    </div>
  ),
} satisfies Meta<typeof Blurhash>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Waits until the blur has been drawn. */
async function drawn(root: HTMLElement) {
  await waitFor(() => {
    const canvas = root.querySelector("canvas");
    const pixel = canvas?.getContext("2d")?.getImageData(0, 0, 1, 1).data;
    if (pixel?.[3] !== 255) throw new Error("the blur has not been drawn");
  });
}

/** A blur of 4 × 3 components: the colors of the picture, without its detail. */
export const Default: Story = {
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector("canvas")).toHaveAttribute("aria-hidden", "true");
    await drawn(canvasElement);
  },
};

/** Fewer components (here 3 × 3) draw a softer blur. */
export const Soft: Story = {
  args: { hash: "KJG8_@Dgx]_4V?xuyE%NRj" },
  play: async ({ canvasElement }) => {
    await drawn(canvasElement);
  },
};

/** A hash that does not decode draws nothing: the container's surface stands in, as for media without one. */
export const Invalid: Story = {
  args: { hash: "not a blurhash" },
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector("canvas")).toBeNull();
  },
};
