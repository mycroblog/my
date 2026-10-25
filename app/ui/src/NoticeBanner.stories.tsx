// SPDX-License-Identifier: AGPL-3.0-only
// NoticeBanner stories: the closure notice with its details and links, and a headline alone.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Link } from "./Link";
import { NoticeBanner } from "./NoticeBanner";

const meta = {
  title: "Feedback/NoticeBanner",
  component: NoticeBanner,
  tags: ["no-screenshot"],
  args: { label: "Notice of the server's closure", headline: "This server closes on 1 Nov 2026." },
} satisfies Meta<typeof NoticeBanner>;

export default meta;
type Story = StoryObj<typeof meta>;

export const HeadlineOnly: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("region", { name: "Notice of the server's closure" })).toHaveTextContent("closes on 1 Nov 2026");
  },
};

export const WithDetailsAndActions: Story = {
  args: {
    children: (
      <>
        <p>You can sign in and export your data until 1 Dec 2026.</p>
        <p>Thank you for being here.</p>
      </>
    ),
    actions: (
      <>
        <Link href="/settings/account">Export your data</Link>
        <Link href="/settings/account">Move to another server</Link>
      </>
    ),
  },
  play: async ({ canvas }) => {
    await expect(canvas.getAllByRole("link")).toHaveLength(2);
  },
};
