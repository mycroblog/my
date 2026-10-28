// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent } from "storybook/test";
import { BackLinkHeading, PageHeading } from "./PageHeading";
import { Text } from "./Text";

const meta = {
  title: "Shell/PageHeading",
  component: PageHeading,
  args: { children: "Bookmarks" },
} satisfies Meta<typeof PageHeading>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Visible: Story = {
  play: async ({ canvas }) => {
    const heading = canvas.getByRole("heading", { level: 1, name: "Bookmarks" });
    await expect(heading).toBeVisible();
    // Focus moves to it from script after a navigation; it is not in the tab order.
    await expect(heading).toHaveAttribute("tabindex", "-1");
  },
};

/** On a page whose heading only says where the user is (a timeline): in the accessibility tree, not on screen. */
export const VisuallyHidden: Story = {
  args: { children: "Home timeline", visuallyHidden: true },
  render: (args) => (
    <>
      <PageHeading {...args} />
      <Text as="p">The page starts here; its heading is read but not drawn.</Text>
    </>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("heading", { level: 1, name: "Home timeline" })).toHaveClass("sr-only");
  },
};

/**
 * On a page that is not an item of the navigation (a post): the way back before the heading, for the installed app,
 * which has no browser back button.
 */
export const WithBack: Story = {
  args: { children: "Post", back: { label: "Back", onPress: fn() } },
  play: async ({ args, canvas }) => {
    await expect(canvas.getByRole("heading", { level: 1, name: "Post" })).toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: "Back" }));
    await expect(args.back?.onPress).toHaveBeenCalledOnce();
  },
};

/** A long title in another script wraps. */
export const LongTitle: Story = {
  args: { children: "Einstellungen für Benachrichtigungen über Erwähnungen, Antworten und Reaktionen" },
};

/** A page of the settings: its heading and the way back to the settings under it. */
export const WithBackLink: Story = {
  render: (args) => (
    <BackLinkHeading backHref="#settings" backLabel="Back to settings">
      <PageHeading {...args} />
    </BackLinkHeading>
  ),
  args: { children: "Notifications" },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("link", { name: "Back to settings" })).toHaveAttribute("href", "#settings");
  },
};

/** The spark after the title is decorative: the heading's name stays the text. */
export const WithSpark: Story = {
  args: { children: "Home", spark: "✳" },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("heading", { level: 1, name: "Home" })).toBeInTheDocument();
  },
};
