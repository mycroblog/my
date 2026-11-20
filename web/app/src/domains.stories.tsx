// SPDX-License-Identifier: AGPL-3.0-only

// The moderators' screens of domains, remote servers and the audit log (ADR-0058 §1, ADR-0159; FR-702, FR-704, FR-706,
// FR-716–FR-718), on the fake server (app.samples.tsx), whose domains are spam.example suspended, noisy.example limited
// for a month, mediahog.example's media rejected by a subscription, old.example's suspend lifted and far.example's
// limit expired; whose allowlist holds two domains; whose block-list subscriptions are one in the draft mode with three
// drafts and one automatic that keeps failing; whose remote servers are dead.example, unavailable, and flaky.example;
// and whose audit log holds one entry of every kind of operation. The scenario's moderator (marco) reads every story
// but those of another reader. What follows a press (a suspend's confirmation with the follows it ends, an import's
// preview, a draft accepted, a delivery's hidden fields shown) is the route tests' and the E2E's.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const meta = {
  title: "Moderation/Domains",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/moderation/domain-actions", viewer: "marco" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `at` as the scenario's moderator reads it, unless `more` names another reader. */
const at = (path: string, more: Omit<Fake, "at"> = {}) => screen({ at: path, viewer: "marco", ...more });
const none = { server: { domains: "none" as const } };

/** The actions in effect, newest first: each its domain, its public reason, its kind, its end and its state. */
export const DomainActions: Story = { parameters: at("/moderation/domain-actions") };
/** Every action, lifted and expired ones too. */
export const DomainActionsAll: Story = { parameters: at("/moderation/domain-actions?active=all") };
/** Every domain action, on its second page, two a page. */
export const DomainActionsSecondPage: Story = {
  parameters: at("/moderation/domain-actions?active=all", { server: { staffPage: 2 }, next: true }),
};
/** The actions on one domain, as a remote server's screen leads to them. */
export const DomainActionsOfDomain: Story = {
  parameters: at("/moderation/domain-actions?domain=old.example&active=all"),
};
/** No action is in effect: the list says so, and what comes there. */
export const DomainActionsEmpty: Story = { parameters: at("/moderation/domain-actions", none) };
export const DomainActionsLoading: Story = { parameters: at("/moderation/domain-actions", { state: "loading" }) };
export const DomainActionsError: Story = { parameters: at("/moderation/domain-actions", { state: "error" }) };
/** The list's address, for a reader whose permissions do not open it. */
export const DomainActionsNotOpen: Story = { parameters: at("/moderation/domain-actions", { viewer: "yuki" }) };

/** A suspend in effect: its reasons, Lift, and the ways to the server, the domain's actions and the audit log. */
export const DomainAction: Story = { parameters: at("/moderation/domain-actions/{domain:action:spam}") };
/** A lifted suspend that hid part of the domain where it was published. */
export const DomainActionLifted: Story = { parameters: at("/moderation/domain-actions/{domain:action:old}") };
/** An action a block-list subscription took, which leads to the subscription. */
export const DomainActionFromList: Story = { parameters: at("/moderation/domain-actions/{domain:action:media}") };
/** An address that names no action. */
export const DomainActionMissing: Story = { parameters: at("/moderation/domain-actions/none") };

/** The form of a new action, a suspend until lifted at first. */
export const NewDomainAction: Story = { parameters: at("/moderation/domain-actions/new") };
/** The form, the domain filled in, as a remote server's screen opens it. */
export const NewDomainActionOnDomain: Story = { parameters: at("/moderation/domain-actions/new?domain=bread.example") };

/** Mastodon's CSV: Save as CSV, and the import's file before one is chosen. */
export const DomainImport: Story = { parameters: at("/moderation/domain-actions/import") };

/** The allowlist: the form that allows a domain, and the two allowed, each picked to be removed. */
export const DomainAllows: Story = { parameters: at("/moderation/domain-actions/allowed") };
/** No domain is allowed. */
export const DomainAllowsEmpty: Story = { parameters: at("/moderation/domain-actions/allowed", none) };

/** The subscriptions: one automatic that failed three times in a row, one in the draft mode. */
export const DomainLists: Story = { parameters: at("/moderation/domain-actions/subscriptions") };
/** The subscriptions' second page, one a page. */
export const DomainListsSecondPage: Story = {
  parameters: at("/moderation/domain-actions/subscriptions", { server: { staffPage: 1 }, next: true }),
};
/** No subscription yet. */
export const DomainListsEmpty: Story = { parameters: at("/moderation/domain-actions/subscriptions", none) };
/** The form of a new subscription. */
export const NewDomainList: Story = { parameters: at("/moderation/domain-actions/subscriptions/new") };
/** A subscription in the draft mode: its three drafts (a lift first), its fetches, its settings, Stop subscribing. */
export const DomainList: Story = { parameters: at("/moderation/domain-actions/subscriptions/{domain:list:shared}") };
/** A subscription with Show more pressed once under its fetches, two a page. */
export const DomainListMore: Story = {
  parameters: at("/moderation/domain-actions/subscriptions/{domain:list:shared}", {
    server: { staffPage: 2 },
    more: true,
  }),
};
/** An automatic subscription whose last three fetches failed, and no draft waits. */
export const DomainListFailing: Story = {
  parameters: at("/moderation/domain-actions/subscriptions/{domain:list:failing}"),
};
/** An address that names no subscription. */
export const DomainListMissing: Story = { parameters: at("/moderation/domain-actions/subscriptions/none") };

/** The servers deliveries failed to reach, with Open a server. */
export const RemoteServers: Story = { parameters: at("/moderation/instances") };
/** The remote servers' second page, one a page. */
export const RemoteServersSecondPage: Story = {
  parameters: at("/moderation/instances", { server: { staffPage: 1 }, next: true }),
};
/** The servers marked unavailable. */
export const RemoteServersUnavailable: Story = { parameters: at("/moderation/instances?state=unavailable") };
/** Deliveries reach every server. */
export const RemoteServersEmpty: Story = { parameters: at("/moderation/instances", none) };
export const RemoteServersLoading: Story = { parameters: at("/moderation/instances", { state: "loading" }) };
/**
 * An unavailable server: its nine failed days, Mark available, Refetch posts, when deliveries stopped and resumed, and
 * its deliveries, one that keeps failing and one masked.
 */
export const RemoteServer: Story = { parameters: at("/moderation/instances/dead.example") };
/**
 * A remote server with Show more pressed once under its availability and its deliveries, one a page: two of each, and
 * Show more gone where the list is read to its end.
 */
export const RemoteServerMore: Story = {
  parameters: at("/moderation/instances/dead.example", { server: { staffPage: 1 }, more: true }),
};
/** A server deliveries reach, which has never stopped. */
export const RemoteServerAvailable: Story = { parameters: at("/moderation/instances/photos.example") };

/** The audit log, newest first: one entry of every kind of operation, by the reader, another moderator, the command line, the server and another server. */
export const AuditLog: Story = { parameters: at("/moderation/audit-log") };
/** The audit log's second page, two entries a page: Previous and Next, and which page it is. */
export const AuditLogSecondPage: Story = {
  parameters: at("/moderation/audit-log", { server: { staffPage: 2 }, next: true }),
};
/** The operations on one domain, as an action's page and a server's screen lead to them. */
export const AuditLogOfTarget: Story = {
  parameters: at("/moderation/audit-log?target_kind=domain&target=spam.example"),
};
/** The operations on domains of one kind. */
export const AuditLogOfAction: Story = {
  parameters: at("/moderation/audit-log?target_kind=domain&action=domain_action_issued"),
};
/** Nothing is recorded yet. */
export const AuditLogEmpty: Story = { parameters: at("/moderation/audit-log", none) };
export const AuditLogLoading: Story = { parameters: at("/moderation/audit-log", { state: "loading" }) };
export const AuditLogError: Story = { parameters: at("/moderation/audit-log", { state: "error" }) };
