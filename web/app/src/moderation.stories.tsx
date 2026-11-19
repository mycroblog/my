// SPDX-License-Identifier: AGPL-3.0-only

// The staff's screens (ADR-0058 §2; FR-709–FR-712, FR-727), on the fake server (app.samples.tsx), whose queue of
// reports holds a spam account with two posts its author deleted since, a remote photographer's repost handled by the
// scenario's moderator (marco, who reads every story but those of another reader), a report another server sent, a
// voice chat room and an event, one an automated review filed and resolved, and one forwarded when filed and resolved.
// The roots: Moderation with the screens a moderator's permissions open, the open reports counted; and either root for
// a reader whose permissions open none of its screens, or none yet. The queue: the open reports, none, filtered with
// and without a match, the resolved ones, the longest waiting first, loading, failing, and its address for a reader it
// is not open to. A report: with its deleted posts; assigned, with a note, and forwardable; another server's; a voice
// chat room's and an event's; resolved by an automated review's report; forwarded and resolved; offered for
// translation where the server may send a report's text; and loading, failing, and an address that names none. What
// follows a press (assigning, a note, resolving, forwarding, a translation shown) is the route tests'.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const meta = {
  title: "Moderation",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/moderation", viewer: "marco" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `at` as the scenario's moderator reads it, unless `more` names another reader. */
const at = (path: string, more: Omit<Fake, "at"> = {}) => screen({ at: path, viewer: "marco", ...more });

/** Moderation for a moderator: Reports, with how many are open. */
export const Root: Story = { parameters: at("/moderation") };
/** Moderation for a reader whose account has no moderation permission: why no screen is open to them. */
export const RootNone: Story = { parameters: at("/moderation", { viewer: "yuki" }) };
/** Administration for the administrator, whose permissions open no screen built yet. */
export const AdminRootNoneYet: Story = { parameters: at("/admin", { viewer: "hana" }) };
/** Administration for a moderator, who has no permission of the server's administration. */
export const AdminRootNone: Story = { parameters: at("/admin") };

/** The open reports, the newest first: each its account, who filed it, its category, when, and who handles it. */
export const Queue: Story = { parameters: at("/moderation/reports") };
/** No report is open: the queue says so, and what comes there. */
export const QueueEmpty: Story = { parameters: at("/moderation/reports", { server: { reports: "none" } }) };
/** Every report the reader handles, open or resolved: the filters applied, and Clear filters. */
export const QueueMine: Story = { parameters: at("/moderation/reports?state=any&assignee=me") };
/** The open reports of illegal content, of which there is none: the queue says so, with Clear filters. */
export const QueueNoMatch: Story = { parameters: at("/moderation/reports?category=legal") };
/** The resolved reports. */
export const QueueResolved: Story = { parameters: at("/moderation/reports?state=resolved") };
/** The open reports, the longest waiting first, as the column's header says. */
export const QueueOldest: Story = { parameters: at("/moderation/reports?order=oldest") };
export const QueueLoading: Story = { parameters: at("/moderation/reports", { state: "loading" }) };
export const QueueError: Story = { parameters: at("/moderation/reports", { state: "error" }) };
/** The queue's address, for a reader whose permissions do not open it: it names no screen open to them. */
export const QueueNotOpen: Story = { parameters: at("/moderation/reports", { viewer: "yuki" }) };

/**
 * A spam account's report, open, which nobody handles: its post, and two its author deleted since, each with when;
 * the account's earlier action; no note yet; the queue beside it from `wide`.
 */
export const Report: Story = { parameters: at("/moderation/reports/{report:deals}") };
/**
 * A remote photographer's repost, which the reader handles: the rule it cites, the reporter's comment in Japanese, the
 * reader's note, and the way to send it to the account's server.
 */
export const ReportAssigned: Story = { parameters: at("/moderation/reports/{report:photo}") };
/** The same report as the administrator reads it: another moderator handles it. */
export const ReportAssignedToAnother: Story = {
  parameters: at("/moderation/reports/{report:photo}", { viewer: "hana" }),
};
/**
 * The same, on a server that may send a report's text to its provider: Translate under the comment and under the post,
 * which is in another language than the reader's.
 */
export const ReportTranslatable: Story = {
  parameters: at("/moderation/reports/{report:photo}", {
    server: { translation: { provider: "answers", reports: true } },
  }),
};
/** A report another server sent of a local account: who sent it; it cannot be forwarded. */
export const ReportFromAnotherServer: Story = { parameters: at("/moderation/reports/{report:flag}") };
/**
 * A voice chat room's report, which the reader handles: the room's settings as reported, how it ended, who took part
 * and who was removed, and the changes of its settings, a cited rule retired since among the rules.
 */
export const ReportVoiceRoom: Story = { parameters: at("/moderation/reports/{report:room}") };
/** An event's report: its settings as the reporter saw them, its date, and every change, one after the report. */
export const ReportEvent: Story = { parameters: at("/moderation/reports/{report:event}") };
/** A report an automated review filed, resolved with an action: Reopen in the place of the actions. */
export const ReportResolved: Story = { parameters: at("/moderation/reports/{report:review}") };
/** A remote account's report sent to its server when it was filed, and resolved with no action, with two notes. */
export const ReportForwarded: Story = { parameters: at("/moderation/reports/{report:copied}") };
export const ReportLoading: Story = { parameters: at("/moderation/reports/{report:deals}", { state: "loading" }) };
export const ReportError: Story = { parameters: at("/moderation/reports/{report:deals}", { state: "error" }) };
/** An address that names no report. */
export const ReportMissing: Story = { parameters: at("/moderation/reports/none") };
