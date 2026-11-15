// SPDX-License-Identifier: AGPL-3.0-only

// The app's shell around its screens (ADR-0110 §11, ADR-0125 §4, ADR-0088, ADR-0028 §11–§12, ADR-0152), on the fake
// server (app.samples.tsx), in each state of the design system's Shell stories: a reader at home with every section
// around them; the notifications, where no new post floats; staff, whose places end with moderation (its count), admin
// and finance; a server with its local and federated timelines off; a one-person server with its logo, for its
// operator and for a visitor, who opens on the operator's profile; the local timeline, for a reader and for a visitor
// (who opens any other server on the landing page, landing.stories.tsx); the closure announced, and the
// server closed with no export, one being built and one ready; the maintenance; the reader offline and the server down;
// a call; the toasts of a newer version and of one the server needs; the voice rooms off; the column beside the main
// one with none of its sections; and the menu open.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, waitFor } from "storybook/test";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";
import { callAcrossScreens } from "./shell.plays";

const meta = {
  title: "Shell",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Home: Story = {};
export const Notifications: Story = { parameters: screen({ at: "/notifications" }) };
export const Staff: Story = { parameters: screen({ at: "/notifications", viewer: "hana" }) };
export const TimelinesOff: Story = {
  parameters: screen({ at: "/profile", server: { timelines: { local: "off", federated: "off" } } }),
};
export const SingleUser: Story = { parameters: screen({ at: "/", server: { singleUser: true, logo: true } }) };
export const SingleUserVisitor: Story = {
  parameters: screen({ at: "/", signedOut: true, server: { singleUser: true, logo: true } }),
};
export const Local: Story = { parameters: screen({ at: "/local" }) };
/** The local timeline for a visitor: the posts' actions show, and those that need an account lead to sign in. */
export const LocalVisitor: Story = { parameters: screen({ at: "/local", signedOut: true }) };
export const ClosureAnnounced: Story = { parameters: screen({ at: "/", server: { closure: "announced" } }) };
export const Closed: Story = { parameters: screen({ at: "/", server: { closure: "closed" } }) };
/** Closed, with the archive of the reader's data being built: the request waits. */
export const ClosedExportBuilding: Story = {
  parameters: screen({ at: "/", server: { closure: "closed", dataExport: "building" } }),
};
/** Closed, with the archive ready to download until it expires. */
export const ClosedExportReady: Story = {
  parameters: screen({ at: "/", server: { closure: "closed", dataExport: "ready" } }),
};
export const Maintenance: Story = { parameters: screen({ at: "/", server: { maintenance: true } }) };
export const Offline: Story = {
  parameters: screen({ at: "/", server: { closure: "announced", down: "offline" } }),
};
export const Outage: Story = { parameters: screen({ at: "/notifications", server: { down: "outage" } }) };
/**
 * A call, with a newer build waiting that no toast offers during it. Its play mutes and unmutes from the call's bar,
 * then moves to the notifications and back: the bar stays across the screens.
 */
export const InCall: Story = {
  parameters: screen({ at: "/", call: true, updateReady: true }),
  play: callAcrossScreens,
};
export const UpdateReady: Story = { parameters: screen({ at: "/", updateReady: true }) };
export const UpdateRequired: Story = { parameters: screen({ at: "/notifications", server: { newerEpoch: true } }) };
/** The voice rooms off: no section of them beside the timeline, and no place of them in the menu. */
export const VoiceOff: Story = { parameters: screen({ at: "/", server: { voiceOff: true } }) };
export const AroundEmpty: Story = { parameters: screen({ at: "/", server: { around: "none" } }) };

/** Waits until the menu has risen into place, so that the story tests check it as the reader sees it. */
async function menuRisen(doc: Document) {
  await waitFor(
    () => {
      const panel = doc.querySelector('[role="dialog"]')?.parentElement;
      const style = panel === null || panel === undefined ? undefined : getComputedStyle(panel);
      if (style?.opacity !== "1" || !["none", "matrix(1, 0, 0, 1, 0, 0)"].includes(style.transform))
        throw new Error("the menu is still rising");
    },
    { timeout: 5000 },
  );
}

// Menu pressed where it shows, for staff: the tab bar's sheet on a phone, the popover beside the rail's or the
// sidebar's entry from `tablet`.
export const MenuOpen: Story = {
  parameters: screen({ at: "/", viewer: "hana" }),
  play: async (context) => {
    await screenMeta.play(context);
    // The menu is the navigation's one button that opens something, found so in every locale.
    const menus = [...context.canvasElement.querySelectorAll<HTMLElement>("nav button[aria-expanded]")];
    const [menu] = menus.filter((m) => m.getClientRects().length > 0);
    if (menu === undefined) throw new Error("no menu shows");
    await userEvent.click(menu);
    await menuRisen(context.canvasElement.ownerDocument);
  },
};

export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };
