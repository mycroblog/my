// SPDX-License-Identifier: AGPL-3.0-only

// The closure of the service (FR-808, FR-809, ADR-0088), on the fake server (app.samples.tsx) as its administrator
// (hana) reads it: none announced; announced, with a short notice, and with its closing date passed; closed, its
// deliveries going on and ended; the announcement's form, new and changing the dates; the check before closing, with
// its warnings, before the closing date and once it can close; loading and failing. What follows a press (the
// questions, the domain typed to close, what the server refused) is the route tests'.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const meta = {
  title: "Administration/Closure",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/admin/closure", viewer: "hana" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `at` as the scenario's administrator reads it. */
const at = (path: string, more: Omit<Fake, "at"> = {}) => screen({ at: path, viewer: "hana", ...more });

/** No closure announced: Announce the closure. */
export const None: Story = { parameters: at("/admin/closure") };
/** Announced, 60 days ahead: its dates and message, Change the dates and Withdraw, and when it can close. */
export const Announced: Story = { parameters: at("/admin/closure", { server: { closure: "announced" } }) };
/** Announced at less than 30 days' notice: the warning under its facts. */
export const AnnouncedShort: Story = {
  parameters: at("/admin/closure", { server: { closure: "announced", closureAdmin: { shortNotice: true } } }),
};
/** Announced, its closing date passed: Check and close. */
export const AnnouncedDue: Story = {
  parameters: at("/admin/closure", { server: { closure: "announced", closureAdmin: { due: true } } }),
};
/** Closed: what remains, and the progress of the deletion of the accounts on other servers. */
export const Closed: Story = { parameters: at("/admin/closure", { server: { closure: "closed" } }) };
/** Closed, every delivery ended. */
export const Delivered: Story = {
  parameters: at("/admin/closure", { server: { closure: "closed", closureAdmin: { delivered: true } } }),
};
export const Loading: Story = { parameters: at("/admin/closure", { state: "loading" }) };
export const Failed: Story = { parameters: at("/admin/closure", { state: "error" }) };

/** The announcement's form, new: the closing date, the export deadline, and the message. */
export const Announce: Story = { parameters: at("/admin/closure/announcement") };
/** The announcement's form, changing the dates of the one in effect. */
export const ChangeDates: Story = {
  parameters: at("/admin/closure/announcement", { server: { closure: "announced" } }),
};

/** The check before closing, before the closing date: what closing does, and when it can close. */
export const CheckEarly: Story = { parameters: at("/admin/closure/close", { server: { closure: "announced" } }) };
/** The check once the server can close, with moves not ended and a retention hold: Close the server last. */
export const CheckDue: Story = {
  parameters: at("/admin/closure/close", {
    server: { closure: "announced", closureAdmin: { due: true, warnings: true } },
  }),
};
