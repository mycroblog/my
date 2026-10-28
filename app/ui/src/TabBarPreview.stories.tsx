// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { BookmarkIcon, FederatedIcon, HomeIcon, LocalIcon, NotificationsIcon, ProfileIcon, SearchIcon } from "./Icons";
import { TabBarPreview } from "./TabBarPreview";

const meta = {
  title: "Shell/TabBarPreview",
  component: TabBarPreview,
  args: {
    items: [
      { id: "home", icon: <HomeIcon />, label: "Home" },
      { id: "local", icon: <LocalIcon />, label: "Local" },
      { id: "public", icon: <FederatedIcon />, label: "Federated" },
      { id: "notifications", icon: <NotificationsIcon />, label: "Notifications" },
      { id: "search", icon: <SearchIcon />, label: "Search" },
    ],
    menuLabel: "Menu",
    caption: "Tab bar on phones",
  },
} satisfies Meta<typeof TabBarPreview>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The default arrangement: five items, then Menu. The items are text in a list, not links or buttons. */
export const Default: Story = {
  play: async ({ canvas }) => {
    const items = canvas.getAllByRole("listitem");
    await expect(items).toHaveLength(6);
    await expect(items[5]).toHaveTextContent("Menu");
    await expect(canvas.queryByRole("link")).toBeNull();
    await expect(canvas.queryByRole("button")).toBeNull();
    await expect(canvas.getByRole("figure", { name: "Tab bar on phones" })).toBeVisible();
  },
};

/** Fewer items (some hidden, or a timeline the server does not serve): the bar shares its width among them. */
export const FewItems: Story = {
  args: {
    items: [
      { id: "profile", icon: <ProfileIcon />, label: "Profile" },
      { id: "bookmarks", icon: <BookmarkIcon />, label: "Bookmarks" },
    ],
  },
  play: async ({ canvas }) => {
    await expect(canvas.getAllByRole("listitem")).toHaveLength(3);
  },
};
