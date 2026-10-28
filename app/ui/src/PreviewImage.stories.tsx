// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { PreviewImage } from "./PreviewImage";

const image =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 160 90'%3E%3Crect width='160' height='90' fill='%2314b8a6'/%3E%3C/svg%3E";

const meta = {
  title: "Display/PreviewImage",
  component: PreviewImage,
  args: { src: image, alt: "" },
} satisfies Meta<typeof PreviewImage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
