// SPDX-License-Identifier: AGPL-3.0-only

// The announcements as the administrator writes them (FR-726, ADR-0125), on the fake server (app.samples.tsx;
// staff-announcements.ts), whose list holds a draft, one published to start in a few days, one showing now, one whose
// window ended and one withdrawn. The list: every one, kept to each state, none at all, loading, failing, and its
// address for a moderator, whose permissions do not open it. The form of a new one. An announcement's page: a draft
// (Save and Publish), one showing now and one scheduled (Save, and Withdraw last), each with where it stands and what
// readers see, as their own page of it shows it; a withdrawn one, nothing to change; loading, failing, and an address that names none. What follows a
// press (a refused field, the question before publishing or withdrawing) is the route tests'.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const meta = {
  title: "Administration/Announcements",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/admin/announcements", viewer: "hana" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `at` as the administrator reads it. */
const asAdmin = (path: string, more: Omit<Fake, "at"> = {}) => screen({ at: path, viewer: "hana", ...more });

/** Every announcement, the newest first: its title, when it shows and where it stands. */
export const Announcements: Story = { parameters: asAdmin("/admin/announcements") };
/** The announcements' second page, two a page. */
export const AnnouncementsSecondPage: Story = {
  parameters: asAdmin("/admin/announcements", { server: { staffPage: 2 }, next: true }),
};
/** The drafts alone. */
export const AnnouncementsDrafts: Story = { parameters: asAdmin("/admin/announcements?state=draft") };
/** The ones waiting for their window to start. */
export const AnnouncementsScheduled: Story = { parameters: asAdmin("/admin/announcements?state=scheduled") };
/** The ones every local user sees now. */
export const AnnouncementsShowing: Story = { parameters: asAdmin("/admin/announcements?state=showing") };
/** The ones whose window ended. */
export const AnnouncementsEnded: Story = { parameters: asAdmin("/admin/announcements?state=ended") };
/** The withdrawn ones. */
export const AnnouncementsWithdrawn: Story = { parameters: asAdmin("/admin/announcements?state=withdrawn") };
/** None written yet: the list says so, and what comes there. */
export const AnnouncementsEmpty: Story = {
  parameters: asAdmin("/admin/announcements", { server: { staffServer: { announcements: { start: "none" } } } }),
};
/** None in the state asked for, with Clear filters. */
export const AnnouncementsNoMatch: Story = {
  parameters: asAdmin("/admin/announcements?state=draft", {
    server: { staffServer: { announcements: { start: "none" } } },
  }),
};
export const AnnouncementsLoading: Story = { parameters: asAdmin("/admin/announcements", { state: "loading" }) };
export const AnnouncementsError: Story = { parameters: asAdmin("/admin/announcements", { state: "error" }) };
/** The list's address, for a moderator, whose permissions do not open it. */
export const AnnouncementsNotOpen: Story = { parameters: screen({ at: "/admin/announcements", viewer: "marco" }) };

/** A new announcement: its title, its text in the editor, when it shows, Save as draft and Publish. */
export const NewAnnouncement: Story = { parameters: asAdmin("/admin/announcements/new") };

/**
 * A draft: where it stands, its form, what readers see as last saved (their own page of it, as if it were published
 * now), and Save and Publish; no Withdraw.
 */
export const Draft: Story = { parameters: asAdmin("/admin/announcements/{server:announcement:draft}") };
/** One every local user sees now: its form and Save, what readers see, and Withdraw last. */
export const Showing: Story = { parameters: asAdmin("/admin/announcements/{server:announcement:showing}") };
/** One published that starts in a few days and ends a few days later. */
export const Scheduled: Story = { parameters: asAdmin("/admin/announcements/{server:announcement:scheduled}") };
/** One whose window ended, still revised and withdrawn as a published one is. */
export const Ended: Story = { parameters: asAdmin("/admin/announcements/{server:announcement:ended}") };
/** A withdrawn one: where it stands and what it said, nothing to change. */
export const Withdrawn: Story = { parameters: asAdmin("/admin/announcements/{server:announcement:withdrawn}") };
export const AnnouncementLoading: Story = {
  parameters: asAdmin("/admin/announcements/{server:announcement:draft}", { state: "loading" }),
};
export const AnnouncementError: Story = {
  parameters: asAdmin("/admin/announcements/{server:announcement:draft}", { state: "error" }),
};
/** An address that names no announcement. */
export const AnnouncementMissing: Story = { parameters: asAdmin("/admin/announcements/none") };
