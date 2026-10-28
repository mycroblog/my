// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { AutoplayContext, Picture } from "./Picture";

/** A stand-in rendition of the given color and width, inline so the story does not depend on the network. */
function rendition(fill: string, width: number) {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${String(width)}" height="${String(width / 2)}" viewBox="0 0 2 1"><rect width="2" height="1" fill="${fill}"/><circle cx="1" cy="0.5" r="0.3" fill="#f4d7b5"/></svg>`;
  return { url: `data:image/svg+xml,${encodeURIComponent(svg)}`, width };
}

const still = [rendition("#3b6fd8", 160), rendition("#3b6fd8", 320), rendition("#3b6fd8", 640)];
const animated = [rendition("#d83b6f", 160), rendition("#d83b6f", 320), rendition("#d83b6f", 640)];

const meta = {
  title: "Display/Picture",
  component: Picture,
  args: { sources: { still }, sizes: "20rem", alt: "A placeholder picture", className: "w-80 max-w-full" },
} satisfies Meta<typeof Picture>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Waits until the image has decoded, so the screenshot shows it. */
async function loaded(root: HTMLElement) {
  await waitFor(() => {
    const img = root.querySelector("img");
    if (img === null || !img.complete || img.naturalWidth === 0) throw new Error("the picture has not loaded");
  });
}

/** A still image: the browser loads the rendition that fits its width. */
export const Still: Story = {
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByRole("img", { name: "A placeholder picture" })).toHaveAttribute("sizes", "20rem");
    await loaded(canvasElement);
  },
};

/**
 * An animation that plays by itself: the moving rendition (red here) applies only without a reduced-motion
 * preference, so the screenshots, taken with reduced motion, show the still one.
 */
export const Animated: Story = {
  args: { sources: { still, animated } },
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector("source")).toHaveAttribute(
      "media",
      "(prefers-reduced-motion: no-preference)",
    );
    await loaded(canvasElement);
  },
};

/** With animations off, the still rendition shows until the pointer is over the picture. */
export const AutoplayOff: Story = {
  args: { sources: { still, animated } },
  render: (args) => (
    <AutoplayContext value={false}>
      <Picture {...args} />
    </AutoplayContext>
  ),
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector("source")).toBeNull();
    await loaded(canvasElement);
  },
};
