// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Heading } from "./Heading";
import { ArchiveIcon, BadgeIcon, ProfileIcon } from "./Icons";
import { NavMenu } from "./NavMenu";
import { SettingsLayout } from "./SettingsLayout";
import { Text } from "./Text";

const menu = (
  <NavMenu
    groups={[
      {
        id: "profile",
        heading: "Profile",
        items: [
          { href: "#profile", icon: <ProfileIcon />, label: "Profile", current: true },
          { href: "#badges", icon: <BadgeIcon />, label: "Badges" },
        ],
      },
      {
        id: "account",
        heading: "Account and data",
        items: [{ href: "#account", icon: <ArchiveIcon />, label: "Archive and deletion" }],
      },
    ]}
  />
);

const meta = {
  title: "Shell/SettingsLayout",
  component: SettingsLayout,
  parameters: { layout: "fullscreen" },
  args: {
    menu,
    menuLabel: "Settings sections",
    view: "pane",
    children: <Text as="p">The section.</Text>,
  },
} satisfies Meta<typeof SettingsLayout>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A section's page: the menu beside it from `md`, the current section is `aria-current`. */
export const Section: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("navigation", { name: "Settings sections" })).toBeInTheDocument();
    await expect(canvas.getByRole("link", { name: "Profile" })).toHaveAttribute("aria-current", "page");
  },
};

/** The page of the settings: its heading, then the menu and the rest of the page, side by side when wide and stacked when narrow. */
export const MenuPage: Story = {
  args: {
    view: "menu",
    heading: <Heading level={1}>Settings</Heading>,
    children: <Text as="p">The account and signing out.</Text>,
  },
  play: async ({ canvas }) => {
    // At every width the page keeps what the menu does not hold.
    await expect(canvas.getByRole("heading", { level: 1, name: "Settings" })).toBeVisible();
    await expect(canvas.getByRole("navigation", { name: "Settings sections" })).toBeVisible();
    await expect(canvas.getByText("The account and signing out.")).toBeVisible();
  },
};
