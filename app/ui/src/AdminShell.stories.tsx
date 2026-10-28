// SPDX-License-Identifier: AGPL-3.0-only
// The frame of the admin and moderation mode at the layouts of the design: the menu beside the page from md, the top bar
// and the sheet below it. The shell answers to the viewport (web-client/README.md D2), so each story sets one (storyViewports.ts).
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within, waitFor } from "storybook/test";
import { AdminNavBack, AdminShell, type AdminShellProps } from "./AdminShell";
import { BrandMark } from "./BrandMark";
import { PageHeading } from "./PageHeading";
import { AccountsIcon, ReportIcon, ServerSettingsIcon } from "./Icons";
import { NavMenu } from "./NavMenu";
import { atViewport, shellParameters } from "./storyViewports";
import { Text } from "./Text";

const navigation = (
  <NavMenu
    groups={[
      {
        id: "moderation",
        heading: "Moderation",
        items: [
          {
            href: "#reports",
            icon: <ReportIcon />,
            label: "Reports",
            current: true,
            badge: { count: 3, label: "3 open", name: "Reports 3 open" },
          },
          { href: "#accounts", icon: <AccountsIcon />, label: "Accounts" },
        ],
      },
      {
        id: "admin",
        heading: "Administration",
        items: [{ href: "#settings", icon: <ServerSettingsIcon />, label: "Server settings" }],
      },
    ]}
  />
);

function Sample(props: Partial<AdminShellProps> & { startOpen?: boolean }) {
  const [open, setOpen] = useState(props.startOpen ?? false);
  return (
    <AdminShell
      brand={<BrandMark wordmarkName="my" instanceName="my.example" size="sm" />}
      modeLabel="Admin and moderation"
      navigation={navigation}
      footer={<AdminNavBack href="#app" label="Back to the app" />}
      menuButtonLabel="Admin menu"
      closeLabel="Close"
      menuOpen={open}
      onMenuOpenChange={setOpen}
      skipLinkLabel="Skip to main content"
      {...props}
    >
      <PageHeading>Reports</PageHeading>
      <Text as="p">The screen.</Text>
    </AdminShell>
  );
}

const meta = {
  title: "Shell/AdminShell",
  component: Sample,
  parameters: shellParameters,
} satisfies Meta<typeof Sample>;

export default meta;
type Story = StoryObj<typeof meta>;

/** From md: the menu beside the page, the current screen is `aria-current`, the way back at its foot. */
export const Desktop: Story = {
  globals: atViewport("laptop"),
  play: async ({ canvas }) => {
    const nav = canvas.getByRole("navigation", { name: "Admin and moderation" });
    await expect(within(nav).getByRole("link", { name: "Reports 3 open" })).toHaveAttribute("aria-current", "page");
    await expect(within(nav).getByRole("link", { name: "Back to the app" })).toBeVisible();
  },
};

/** The tablet width has the same menu. */
export const Tablet: Story = { globals: atViewport("tablet") };

/** Below md: the top bar with the menu button, and the page. */
export const Phone: Story = {
  globals: atViewport("phone"),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("button", { name: "Admin menu" })).toBeVisible();
    await expect(canvas.queryByRole("navigation")).not.toBeInTheDocument();
  },
};

/** Below md, the button opens the menu as a sheet. */
export const PhoneMenuOpen: Story = {
  globals: atViewport("phone"),
  render: () => <Sample startOpen />,
  play: async ({ canvas }) => {
    const sheet = await canvas.findByRole("dialog", { name: "Admin menu" });
    // The sheet fades in; wait for it rather than catching it mid-transition.
    await waitFor(() => expect(within(sheet).getByRole("link", { name: "Back to the app" })).toBeVisible());
    await userEvent.keyboard("{Escape}");
  },
};
