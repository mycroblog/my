// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent } from "storybook/test";
import { Disclosure } from "./Disclosure";

const meta = {
  title: "Display/Disclosure",
  component: Disclosure,
  args: {
    summary: "How the server says it",
    children: <p>The server says it in its robots.txt and in the headers of every page.</p>,
  },
} satisfies Meta<typeof Disclosure>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Folded until the summary is pressed. */
export const Default: Story = {
  play: async ({ canvas }) => {
    const text = canvas.getByText(/robots\.txt/);
    await expect(text).not.toBeVisible();
    await userEvent.click(canvas.getByText("How the server says it"));
    await expect(text).toBeVisible();
  },
};
