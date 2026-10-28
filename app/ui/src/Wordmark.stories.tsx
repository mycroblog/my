// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { BrandMark } from "./BrandMark";
import { Wordmark } from "./Wordmark";

const meta = {
  title: "Display/Wordmark",
  component: Wordmark,
  args: { name: "my", decorative: false },
} satisfies Meta<typeof Wordmark>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Waits until every visible image has decoded, so the screenshot shows it. */
async function imagesLoaded(root: HTMLElement) {
  await waitFor(() => {
    for (const img of root.querySelectorAll("img")) {
      if (img.offsetParent !== null && (!img.complete || img.naturalWidth === 0)) throw new Error("not loaded");
    }
  });
}

/** Coral on a light ground, white on a dark one (use the theme toolbar); a standalone logo is an image named "my". */
export const Standalone: Story = {
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByRole("img", { name: "my" })).toBeVisible();
    await imagesLoaded(canvasElement);
  },
};

/** The three heights. */
export const Sizes: Story = {
  render: (args) => (
    <div className="flex items-end gap-6">
      <Wordmark {...args} size="sm" />
      <Wordmark {...args} size="md" />
      <Wordmark {...args} size="lg" />
    </div>
  ),
  play: async ({ canvasElement }) => {
    await imagesLoaded(canvasElement);
  },
};

/** The single charcoal ink, for a ground that is neither light nor dark. */
export const Monochrome: Story = {
  args: { tone: "monochrome" },
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByRole("img", { name: "my" })).toBeVisible();
    await imagesLoaded(canvasElement);
  },
};

/** Next to text that names it, the wordmark is decorative and has no accessible name. */
export const Decorative: Story = {
  args: { decorative: true },
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.queryByRole("img")).toBeNull();
    await imagesLoaded(canvasElement);
  },
};

/** The wordmark with the server name. */
export const WithServerName: Story = {
  render: () => <BrandMark wordmarkName="my" instanceName="Example Server" />,
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByRole("img", { name: "my" })).toBeVisible();
    await expect(canvas.getByText("Example Server")).toBeVisible();
    await imagesLoaded(canvasElement);
  },
};

/** A long name truncates inside a narrow space instead of overflowing. */
export const LongServerName: Story = {
  render: () => (
    <div className="w-64">
      <BrandMark wordmarkName="my" instanceName="The Extraordinarily Long Name of a Community Server for Friends" />
    </div>
  ),
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByText(/Extraordinarily/)).toBeVisible();
    await imagesLoaded(canvasElement);
  },
};

/** A right-to-left name keeps its own direction (`dir="auto"`) while the wordmark stays left of it in LTR pages. */
export const RightToLeftServerName: Story = {
  render: () => <BrandMark wordmarkName="my" instanceName="خادم الأصدقاء" />,
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByText("خادم الأصدقاء")).toHaveAttribute("dir", "auto");
    await imagesLoaded(canvasElement);
  },
};
