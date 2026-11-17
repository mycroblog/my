// SPDX-License-Identifier: AGPL-3.0-only

// The app's shell around its screens (ADR-0110 §11, ADR-0125 §4, ADR-0088, ADR-0028 §11–§12, ADR-0152), on the fake
// server (app.samples.tsx), in each state of the design system's Shell stories: a reader at home with every section
// around them; the notifications, where no new post floats; staff, whose places end with moderation (its count), admin
// and finance; a server with its local and federated timelines off; a one-person server with its logo, for its
// operator and for a visitor, who opens on the operator's profile; the local timeline, for a reader and for a visitor
// (who opens any other server on the landing page, landing.stories.tsx); the closure announced, and the
// server closed with no export, one being built and one ready; the maintenance; the reader offline and the server down;
// a call; the toasts of a newer version and of one the server needs; the voice rooms off; the column beside the main
// one with none of its sections. The Menu open and the floating New post's menu open are the parts' own (the design
// system's Shell and Menu everywhere).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

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
/** A call, with a newer build waiting that no toast offers during it. */
export const InCall: Story = { parameters: screen({ at: "/", call: true, updateReady: true }) };
export const UpdateReady: Story = { parameters: screen({ at: "/", updateReady: true }) };
export const UpdateRequired: Story = { parameters: screen({ at: "/notifications", server: { newerEpoch: true } }) };
/** The voice rooms off: no section of them beside the timeline, and no place of them in the menu. */
export const VoiceOff: Story = { parameters: screen({ at: "/", server: { voiceOff: true } }) };
export const AroundEmpty: Story = { parameters: screen({ at: "/", server: { around: "none" } }) };
