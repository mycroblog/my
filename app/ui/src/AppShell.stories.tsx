// SPDX-License-Identifier: AGPL-3.0-only
// The app shell at each of its layouts. The shell answers to the viewport (web-client/README.md D2), so each story sets one
// (storyViewports.ts) and is captured once at it; the other projects add the right-to-left direction and the dark theme.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { AppNav, AppNavAccount, AppNavAction, AppNavBrand, AppNavItem, AppNavMenu, AppNavMenuLink, AppNavMenuList, AppNavMenuSection, type AppNavBadge } from "./AppNavigation";
import { AppShell, AsidePanel, ShellDock } from "./AppShell";
import { Avatar } from "./Avatar";
import { Button } from "./Button";
import { CallBar } from "./CallBar";
import {
  AdminIcon,
  BookmarkIcon,
  FederatedIcon,
  HomeIcon,
  LocalIcon,
  ModerationIcon,
  NotificationsIcon,
  PlusIcon,
  ProfileIcon,
  SearchIcon,
  SettingsIcon,
} from "./Icons";
import { Link } from "./Link";
import { PageHeading } from "./PageHeading";
import { ServerNameLink } from "./ServerNameLink";
import { atViewport, shellParameters } from "./storyViewports";
import { Switch } from "./Switch";
import { Hint, InlineError, Text } from "./Text";

type Labels = { home: string; local: string; federated: string; notifications: string; search: string; profile: string; bookmarks: string; settings: string };

const labels: Labels = {
  home: "Home",
  local: "Local",
  federated: "Federated",
  notifications: "Notifications",
  search: "Search",
  profile: "Profile",
  bookmarks: "Bookmarks",
  settings: "Settings",
};

/** Labels as long as a long translation's, which wrap inside a word rather than being cut. */
const longLabels: Labels = {
  home: "Startseite",
  local: "Lokale Zeitleiste",
  federated: "Föderierte Zeitleiste",
  notifications: "Benachrichtigungen",
  search: "Durchsuchen",
  profile: "Profil",
  bookmarks: "Lesezeichen",
  settings: "Einstellungen",
};

const unread: AppNavBadge = { count: 3, label: "3 unread notifications", name: "Notifications, 3 unread" };
const pending: AppNavBadge = { count: 12, label: "12 pending", name: "Moderation, 12 pending" };

function Brand({ name }: { name: string }) {
  return (
    <AppNavBrand>
      <ServerNameLink name={name} description="The server's top page" size="lg" />
    </AppNavBrand>
  );
}

/** The navigation of people who are not signed in: a light top bar with the server's pages and the way in. */
function SignedOutNav() {
  return (
    <AppNav label="Main" placement="top" header={<Brand name="my example" />}>
      <AppNavItem href="#about" icon={<HomeIcon />} label="About this server" current />
      <AppNavItem href="#rules" icon={<SearchIcon />} label="Rules" />
      <AppNavItem href="#login" icon={<ProfileIcon />} label="Log in" />
      <AppNavItem href="#signup" icon={<PlusIcon />} label="Create an account" prominent />
    </AppNav>
  );
}

type SampleNavProps = {
  labels?: Labels;
  serverName?: string;
  accountName?: string;
  /** Opens the menu's sheet from the start. */
  menuOpen?: boolean;
};

/** The navigation as the app builds it: five items in the tab bar, the rest in the rail and the menu's sheet. */
function SampleNav({ labels: l = labels, serverName = "my example", accountName = "Alice Example", menuOpen = false }: SampleNavProps) {
  const [isOpen, setIsOpen] = useState(menuOpen);
  return (
    <AppNav
      label="Main"
      header={<Brand name={serverName} />}
      action={<AppNavAction label="Write a post" icon={<PlusIcon />} onPress={() => undefined} />}
      footer={
        <AppNavAccount
          href="#profile"
          avatar={<Avatar name={accountName} size="sm" decorative />}
          name={accountName}
          handle="@alice"
          description="Your profile"
        />
      }
    >
      <AppNavItem href="#home" icon={<HomeIcon />} label={l.home} current />
      <AppNavItem href="#local" icon={<LocalIcon />} label={l.local} />
      <AppNavItem href="#public" icon={<FederatedIcon />} label={l.federated} />
      <AppNavItem href="#notifications" icon={<NotificationsIcon />} label={l.notifications} badge={unread} />
      <AppNavItem href="#search" icon={<SearchIcon />} label={l.search} />
      <AppNavItem href="#profile" icon={<ProfileIcon />} label={l.profile} shows="rail" />
      <AppNavItem href="#bookmarks" icon={<BookmarkIcon />} label={l.bookmarks} shows="rail" />
      <AppNavItem href="#settings" icon={<SettingsIcon />} label={l.settings} shows="rail" />
      <AppNavItem href="#moderation" icon={<ModerationIcon />} label="Moderation" shows="rail" badge={pending} />
      <AppNavItem href="#admin" icon={<AdminIcon />} label="Administration" shows="rail" />
      <AppNavMenu
        label="Menu"
        badge={{ count: 12, label: "12 pending", name: "Menu, 12 pending in moderation" }}
        isOpen={isOpen}
        onOpenChange={setIsOpen}
        title="Menu"
        closeLabel="Close"
      >
        <AppNavMenuList>
          <AppNavMenuLink href="#profile" icon={<ProfileIcon />} label={l.profile} />
          <AppNavMenuLink href="#bookmarks" icon={<BookmarkIcon />} label={l.bookmarks} />
          <AppNavMenuLink href="#settings" icon={<SettingsIcon />} label={l.settings} current />
          <AppNavMenuLink href="#moderation" icon={<ModerationIcon />} label="Moderation" badge={pending} />
          <AppNavMenuLink href="#admin" icon={<AdminIcon />} label="Administration" />
        </AppNavMenuList>
        <AppNavMenuSection separator="4">
          <Hint>The color theme switch goes here.</Hint>
        </AppNavMenuSection>
        <AppNavMenuSection separator="3">
          <ServerNameLink name={serverName} description="The server's top page" />
        </AppNavMenuSection>
        <AppNavMenuSection>
          <Text as="p" size="sm">
            <Link href="#about" variant="standalone">
              More about this server
            </Link>
          </Text>
        </AppNavMenuSection>
      </AppNavMenu>
    </AppNav>
  );
}

/** A short page, so that the shell fits the story's viewport. */
function SamplePage() {
  return (
    <>
      <PageHeading>Home</PageHeading>
      <Text as="p">Posts from the people you follow appear here.</Text>
    </>
  );
}

/** A short page with the bar of a call docked at its end, with a notice of what failed when `failed`. */
function SamplePageInCall({ failed = false }: { failed?: boolean }) {
  return (
    <>
      <SamplePage />
      <ShellDock>
        <CallBar
          label="Voice chat call"
          title={
            <Link href="#room" variant="standalone">
              Friday night radio
            </Link>
          }
          status="Connected"
          notice={failed ? <InlineError alert>No microphone was found. Connect one, then try again.</InlineError> : undefined}
        >
          <Switch defaultSelected>Microphone</Switch>
          <Button variant="danger">Leave</Button>
        </CallBar>
      </ShellDock>
    </>
  );
}

/** A stand-in for the new-post button, whose own look is not the shell's. */
const floatingButton = (
  <Button variant="primary" aria-label="New post">
    <PlusIcon />
  </Button>
);

/** The third column: the server's information. */
const aside = (
  <AsidePanel heading="About this server">
    <ServerNameLink name="my example" description="The server's top page" size="lg" />
    <Hint>Handles on this server end with @my.example</Hint>
    <Text as="p" size="sm">
      <Link href="#about" variant="standalone">
        More about this server
      </Link>
    </Text>
  </AsidePanel>
);

const meta = {
  title: "Shell/AppShell",
  component: AppShell,
  parameters: shellParameters,
  args: {
    navigation: <SampleNav />,
    children: <SamplePage />,
    skipLinkLabel: "Skip to main content",
  },
} satisfies Meta<typeof AppShell>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Phones: the tab bar on the bottom edge with five items and Menu; the counts are in the badges and the names. */
export const Phone: Story = {
  globals: atViewport("phone"),
  play: async ({ canvas }) => {
    const nav = canvas.getByRole("navigation", { name: "Main" });
    await expect(within(nav).getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
    await expect(within(nav).getByRole("link", { name: "Notifications, 3 unread" })).toBeVisible();
    await expect(within(nav).getByRole("button", { name: "Menu, 12 pending in moderation" })).toBeVisible();
    // The rail's items, the server's name and the account are not in the tab bar.
    await expect(within(nav).queryByRole("link", { name: "Bookmarks" })).toBeNull();
    await expect(within(nav).queryByRole("link", { name: "my example" })).toBeNull();
    await expect(canvas.getByRole("main")).not.toHaveAttribute("data-compose-fab");
  },
};

/** Phones on a page with a feed: the floating button above the tab bar, and the page's end clear of both. */
export const PhoneWithFloatingButton: Story = {
  globals: atViewport("phone"),
  args: { floating: floatingButton, floatingVisible: true },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("button", { name: "New post" })).toBeVisible();
    await expect(canvas.getByRole("main")).toHaveAttribute("data-compose-fab", "");
  },
};

/** Phones in a call: the call bar above the tab bar, leaving the corner to the floating button. */
export const PhoneInCall: Story = {
  globals: atViewport("phone"),
  args: { children: <SamplePageInCall />, floating: floatingButton, floatingVisible: true },
  play: async ({ canvas }) => {
    const bar = canvas.getByRole("region", { name: "Voice chat call" });
    await expect(bar).toBeVisible();
    // The bar is part of the page, after its content in the focus order.
    await expect(canvas.getByRole("main")).toContainElement(bar);
  },
};

/**
 * A short screen in a call (320 × 225 at 400% zoom) with a failure: the bar does not stick but ends the page, so that
 * what is fixed (the tab bar and the floating button) stays within half the screen's height.
 */
export const ShortScreenInCall: Story = {
  globals: atViewport("zoomed"),
  args: { children: <SamplePageInCall failed />, floating: floatingButton, floatingVisible: true },
  play: async ({ canvas }) => {
    const bar = canvas.getByRole("region", { name: "Voice chat call" });
    await expect(canvas.getByRole("alert")).toHaveTextContent("No microphone was found.");
    const dock = bar.parentElement;
    if (dock === null) throw new Error("no dock");
    await expect(getComputedStyle(dock).position).toBe("static");
    const nav = canvas.getByRole("navigation", { name: "Main" });
    const fab = canvas.getByRole("button", { name: "New post" });
    await expect(nav.getBoundingClientRect().height + fab.getBoundingClientRect().height).toBeLessThanOrEqual(window.innerHeight / 2);
  },
};

/** Tablets in a call: the call bar at the bottom of the page's column. */
export const TabletInCall: Story = {
  globals: atViewport("tablet"),
  args: { children: <SamplePageInCall />, floating: floatingButton, floatingVisible: true },
};

/** A short screen (a phone in landscape, 400% zoom): the smaller tab bar and the button closer to it. */
export const ShortScreen: Story = {
  globals: atViewport("short"),
  args: { floating: floatingButton, floatingVisible: true },
};

/** The menu's sheet, open: the rail's items with the current one and their counts, then the server's links. */
export const PhoneMenuOpen: Story = {
  globals: atViewport("phone"),
  args: { navigation: <SampleNav menuOpen /> },
  play: async ({ canvas }) => {
    const sheet = await canvas.findByRole("dialog", { name: "Menu" });
    await expect(within(sheet).getByRole("link", { name: "Settings" })).toHaveAttribute("aria-current", "page");
    await waitFor(() => expect(within(sheet).getByRole("link", { name: "Moderation, 12 pending" })).toBeVisible());
  },
};

/** Tablets (md): the rail of icons with short labels, every item and its icon-only new-post button. */
export const Tablet: Story = {
  globals: atViewport("tablet"),
  args: { floating: floatingButton, floatingVisible: true },
  play: async ({ canvas }) => {
    const nav = canvas.getByRole("navigation", { name: "Main" });
    await expect(within(nav).getByRole("link", { name: "Moderation, 12 pending" })).toBeVisible();
    await expect(within(nav).queryByRole("button", { name: /^Menu/ })).toBeNull();
    await expect(within(nav).getByRole("button", { name: "Write a post" })).toBeVisible();
  },
};

/** Laptops (lg): the sidebar with full labels, the server's name above the list and the account below it. */
export const Laptop: Story = {
  globals: atViewport("laptop"),
  args: { floating: floatingButton, floatingVisible: true },
  play: async ({ canvas }) => {
    const nav = canvas.getByRole("navigation", { name: "Main" });
    await expect(within(nav).getByRole("link", { name: "my example" })).toBeVisible();
    await expect(within(nav).getByRole("link", { name: "Alice Example @alice" })).toHaveAccessibleDescription("Your profile");
  },
};

/** Desktops (xl): the third column with the new-post form and the server's information; nothing floats. */
export const Desktop: Story = {
  globals: atViewport("desktop"),
  args: { aside },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("complementary", { name: "About this server" })).toBeVisible();
  },
};

/** Long labels, a long server name and a long display name: labels wrap, names wrap or are cut to one line. */
export const LongNamesPhone: Story = {
  globals: atViewport("phone"),
  args: { navigation: <SampleNav labels={longLabels} serverName="Die außerordentlich gemütliche Gemeinschaftsinstanz" accountName="Alexandra Konstantinopoulou-Wolkenstein" /> },
};

/** The same long names in the sidebar. */
export const LongNamesLaptop: Story = {
  globals: atViewport("laptop"),
  args: { navigation: <SampleNav labels={longLabels} serverName="Die außerordentlich gemütliche Gemeinschaftsinstanz" accountName="Alexandra Konstantinopoulou-Wolkenstein" /> },
};

/** The skip link, the page's first focusable element, shows while it has focus. */
export const SkipLinkFocused: Story = {
  globals: atViewport("phone"),
  play: async ({ canvas }) => {
    await userEvent.tab();
    const skip = canvas.getByRole("link", { name: "Skip to main content" });
    await expect(skip).toHaveFocus();
    await expect(skip).toHaveAttribute("href", "#main");
  },
};

/** Signed out: the top bar in place of the rail and the tab bar, on a tablet. */
export const SignedOutTablet: Story = {
  globals: atViewport("tablet"),
  args: { navigation: <SignedOutNav />, navigationPlacement: "top" },
  play: async ({ canvas }) => {
    const nav = canvas.getByRole("navigation", { name: "Main" });
    await expect(within(nav).getByRole("link", { name: "About this server" })).toHaveAttribute("aria-current", "page");
    await expect(within(nav).getByRole("link", { name: "Create an account" })).toBeVisible();
  },
};

/** Signed out on a phone: the links wrap under the brand instead of scrolling sideways. */
export const SignedOutPhone: Story = {
  globals: atViewport("phone"),
  args: { navigation: <SignedOutNav />, navigationPlacement: "top" },
};
