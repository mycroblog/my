// SPDX-License-Identifier: AGPL-3.0-only

// Account and data, the entry of the settings (FR-715, ADR-0058 §9, §10, ADR-0110 §5), with its first part, the
// account's status, and the two screens under it, on the fake server as yuki reads them. The entry: one action in
// effect, which the summary says with its door; several; none, where a row says so and leads to the same page; while
// the actions load, and when they could not be read. Account status: the fake's limit in effect, which can be appealed,
// over a past action whose appeal lifted it; several in effect, one appealable, one whose appeal waits, one whose time
// to appeal has passed, over past ones decided either way; a past list longer than a page; no action ever; loading, and
// failed. One action: the appeal form, empty (what the reader types there fills it; a shot of it filled is taken by a
// script that types); an action whose appeal waits; one the moderators kept, with their answer; one they lifted; one
// whose time to appeal has passed; one about posts, two that open and one that is gone; on a server that refuses the
// appeal because the time has passed, where Send appeal ends the form: the appeal's status says so with the way to the
// moderators, and what was written stays to be copied; on one that never answers, where Send appeal stays busy; and an
// address that names no action.

import type { FakeOptions } from "@my/e2e/fake";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const entry = "/settings/account";
const status = "/settings/account/status";

type Seed = NonNullable<NonNullable<FakeOptions["settings"]>["moderation"]>[number];

/** An action that keeps the account from posting, in effect for two more weeks, which can be appealed for four days. */
const noPosting: Seed = {
  kind: "restrict_posting",
  reason_category: "spam",
  public_reason: "The same link was posted to forty hashtags within an hour.",
  state: "active",
  issued_at: "-2d",
  expires_at: "+12d",
  appealable: true,
  appeal_deadline: "+4d",
};

/** A limit in effect with no end, whose appeal waits for the moderators. */
const limitWaiting: Seed = {
  kind: "limit",
  reason_category: "violation",
  public_reason: "Repeated replies that took other people's threads off their subject.",
  state: "active",
  issued_at: "-9d",
  appealable: false,
  appeal_deadline: "+171d",
  appeal: {
    id: "waiting",
    text: "I replied to three threads that day, each on its subject. Please look at the replies again.",
    state: "pending",
    submitted_at: "-8d",
  },
};

/** An action in effect that was not appealed, whose time to appeal has passed. */
const sensitivePassed: Seed = {
  kind: "force_sensitive",
  reason_category: "violation",
  public_reason: "Photos of injuries were posted without a content warning several times.",
  state: "active",
  issued_at: "-200d",
  appealable: false,
  appeal_deadline: "-20d",
};

/** A restriction that ended, which the moderators kept when it was appealed, with their answer. */
const interactionUpheld: Seed = {
  kind: "restrict_interaction",
  reason_category: "violation",
  public_reason: "Following and unfollowing hundreds of accounts a day.",
  state: "expired",
  issued_at: "-40d",
  expires_at: "-26d",
  appealable: false,
  appeal_deadline: "+140d",
  appeal: {
    id: "upheld",
    text: "I was looking for people who post about baking and went too fast. I have stopped.",
    state: "upheld",
    submitted_at: "-39d",
    decided_at: "-37d",
    decision_reason: "The follows continued after the first warning, so the restriction stays until it ends.",
  },
};

/** An action on posts, lifted when its appeal was decided: two of the reader's posts, and one that is gone. */
const postsOverturned: Seed = {
  kind: "mark_statuses_sensitive",
  reason_category: "other",
  public_reason: "Photos of a medical procedure without a content warning.",
  ownPosts: 2,
  status_ids: ["01HZZZZZZZZZZZZZZZZZZZZZZZ"],
  state: "lifted",
  issued_at: "-60d",
  lifted_at: "-52d",
  appealable: false,
  appeal: {
    id: "overturned",
    text: "The photos are of my own recovery and I added a warning to each.",
    state: "overturned",
    submitted_at: "-58d",
    decided_at: "-52d",
    decision_reason: "On a second look the posts carried the warning. The action is lifted.",
  },
};

/** A warning that was never appealed. */
const warning: Seed = {
  kind: "warn",
  reason_category: "violation",
  state: "expired",
  issued_at: "-300d",
  expires_at: "-299d",
  appealable: false,
  appeal_deadline: "-120d",
};

const several = {
  settings: { moderation: [noPosting, limitWaiting, sensitivePassed, interactionUpheld, postsOverturned] },
};

/** More past actions than a page of the list holds. */
const manyPast = {
  settings: {
    moderation: Array.from({ length: 24 }, (_, i): Seed => ({
      ...warning,
      public_reason: `Warning ${String(24 - i)}: a reply that named another person's employer.`,
      issued_at: `-${String(10 + i * 9)}d`,
      expires_at: `-${String(9 + i * 9)}d`,
    })),
  },
};

const only = (action: Seed) => ({ settings: { moderation: [action] } });

const meta = {
  title: "Settings account",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: entry }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** /settings/account with one action in effect: the summary says so and leads to Account status. */
export const Entry: Story = {};
/** The entry with three actions in effect. */
export const EntrySeveral: Story = { parameters: screen({ at: entry, server: several }) };
/** The entry with no action in effect: the section Account status, whose row says so and leads to the same page. */
export const EntryClear: Story = { parameters: screen({ at: entry, server: { settings: { moderation: [] } } }) };
/** The entry while the actions are on their way. */
export const EntryLoading: Story = { parameters: screen({ at: entry, state: "loading" }) };
/** The entry when the actions could not be read: it says so, with the way to try again. */
export const EntryFailed: Story = { parameters: screen({ at: entry, state: "error" }) };

/** /settings/account/status: a limit in effect that can be appealed, and a past action whose appeal lifted it. */
export const Status: Story = { parameters: screen({ at: status }) };
/**
 * Three actions in effect, one that can be appealed, one whose appeal waits and one whose time to appeal has passed,
 * over two past ones, one the moderators kept and one they lifted.
 */
export const StatusSeveral: Story = { parameters: screen({ at: status, server: several }) };
/** Twenty-four past actions: the first page of them, and Show more. */
export const StatusPastMany: Story = { parameters: screen({ at: status, server: manyPast }) };
/** No action ever: each list says what would stand there. */
export const StatusEmpty: Story = { parameters: screen({ at: status, server: { settings: { moderation: [] } } }) };
/** Account status while the actions are on their way. */
export const StatusLoading: Story = { parameters: screen({ at: status, state: "loading" }) };
/** Account status when the actions could not be read. */
export const StatusFailed: Story = { parameters: screen({ at: status, state: "error" }) };

/** An action that can be appealed: what it does, the reason, and the appeal form, empty, with the last day to send it. */
export const Action: Story = { parameters: screen({ at: "action:0", server: only(noPosting) }) };
/** An action whose appeal waits for the moderators: where it stands, when it was sent, and what the reader wrote. */
export const ActionWaiting: Story = { parameters: screen({ at: "action:0", server: only(limitWaiting) }) };
/** A past action the moderators kept when it was appealed, with their answer. */
export const ActionUpheld: Story = { parameters: screen({ at: "action:0", server: only(interactionUpheld) }) };
/** A past action on posts the moderators lifted: two posts that open, one that is gone, the appeal and their answer. */
export const ActionOverturned: Story = { parameters: screen({ at: "action:0", server: only(postsOverturned) }) };
/** An action in effect that was never appealed, after the last day to appeal it. */
export const ActionDeadlinePassed: Story = { parameters: screen({ at: "action:0", server: only(sensitivePassed) }) };
/** A warning with no explanation of the moderators. */
export const ActionNoExplanation: Story = { parameters: screen({ at: "action:0", server: only(warning) }) };
/**
 * On a server that refuses the appeal because the time to appeal has passed: after Send appeal, the field and the
 * submit bar are gone, the appeal's status says so with the link to the server's About page, and what was written
 * stays under it.
 */
export const ActionRefused: Story = {
  parameters: screen({ at: "action:0", server: { settings: { moderation: [noPosting], appealSave: "closed" } } }),
};
/** On a server that never answers: Send appeal stays busy, and what was written cannot be changed meanwhile. */
export const ActionSending: Story = {
  parameters: screen({ at: "action:0", server: { settings: { moderation: [noPosting], appealSave: "waits" } } }),
};
/** An address that names no action of the reader's: it says so, with the way to Account status. */
export const ActionNotFound: Story = { parameters: screen({ at: `${status}/nothing` }) };
