// SPDX-License-Identifier: AGPL-3.0-only
// The grouped menu of the settings and of the admin and moderation mode: named groups of pages, an icon on every page,
// a count where something awaits.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { BlockIcon, FollowRequestIcon, MuteIcon, PasskeyIcon, PasswordIcon, ProfileIcon, TwoFactorIcon } from "./Icons";
import { NavMenu } from "./NavMenu";

const meta = {
  title: "Navigation/NavMenu",
  component: NavMenu,
  args: {
    groups: [
      {
        id: "profile",
        heading: "Profile",
        items: [{ href: "#profile", icon: <ProfileIcon />, label: "Profile", current: true }],
      },
      {
        id: "relationships",
        heading: "Relationships",
        items: [
          {
            href: "#follow-requests",
            icon: <FollowRequestIcon />,
            label: "Follow requests",
            badge: { count: 2, label: "2 waiting", name: "Follow requests 2 waiting" },
          },
          { href: "#blocks", icon: <BlockIcon />, label: "Blocked accounts" },
          { href: "#mutes", icon: <MuteIcon />, label: "Muted accounts" },
        ],
      },
      {
        id: "security",
        heading: "Security",
        items: [
          { href: "#password", icon: <PasswordIcon />, label: "Password" },
          { href: "#two-factor", icon: <TwoFactorIcon />, label: "Two-factor authentication" },
          { href: "#passkeys", icon: <PasskeyIcon />, label: "Passkeys" },
        ],
      },
    ],
  },
  decorators: [
    (Story) => (
      <div style={{ maxInlineSize: "16rem" }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof NavMenu>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Each group is named by its heading; the current page is `aria-current`, and a count is in the link's name. */
export const Default: Story = {
  play: async ({ canvas }) => {
    const relationships = canvas.getByRole("group", { name: "Relationships" });
    await expect(within(relationships).getByRole("link", { name: "Follow requests 2 waiting" })).toBeVisible();
    await expect(canvas.getByRole("link", { name: "Profile" })).toHaveAttribute("aria-current", "page");
  },
};
