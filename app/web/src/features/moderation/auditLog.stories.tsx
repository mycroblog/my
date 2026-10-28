// SPDX-License-Identifier: AGPL-3.0-only
// The audit log (/moderation/audit-log; web-client/screens/moderation.md) with its filters, in the states of
// ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  longDomainAccountFixture,
  otherAccountFixture,
  remoteAccountFixture,
  type Schemas,
  viewerFixture,
} from "../../test/fixtures";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";
import { hoursAgo, listing, moderatorViewer, monaAccount, showing, storyId, withoutPermission } from "./test/stories";

function entry(n: number, fields: Omit<Schemas["AdminAuditEntry"], "id" | "occurred_at">): Schemas["AdminAuditEntry"] {
  return { id: storyId("ad", n), occurred_at: hoursAgo(n * 3), ...fields };
}

const byMona = { actor_kind: "user", actor_account_id: monaAccount.id, actor_account: monaAccount } as const;
const byViewer = { actor_kind: "user", actor_account_id: viewerFixture.id, actor_account: viewerFixture } as const;

/** What the staff, the CLI and the server itself did, newest first, a target among them gone since. */
const entries = listing("/admin/audit_log", [
  entry(1, {
    ...byViewer,
    action: "account_action_issued",
    target_kind: "account",
    target_id: otherAccountFixture.id,
    target_label: "@bob",
    target_exists: true,
  }),
  entry(2, {
    ...byViewer,
    action: "report_resolved",
    target_kind: "report",
    target_id: storyId("c0", 6),
    target_label: storyId("c0", 6),
    target_exists: true,
  }),
  entry(3, {
    ...byMona,
    action: "domain_action_issued",
    target_kind: "domain",
    target_id: "spam.example",
    target_label: "spam.example",
    target_exists: true,
  }),
  entry(4, {
    actor_kind: "cli",
    action: "role_granted",
    target_kind: "account",
    target_id: monaAccount.id,
    target_label: "@mona",
    target_exists: true,
  }),
  entry(5, {
    ...byMona,
    action: "revealed_private_data",
    target_kind: "account",
    target_id: remoteAccountFixture.id,
    target_label: "@carol@remote.example",
    target_exists: true,
  }),
  entry(6, {
    actor_kind: "system",
    action: "account_action_lifted",
    target_kind: "account",
    target_id: longDomainAccountFixture.id,
    target_label: `@dmitri@${longDomainAccountFixture.domain}`,
    target_exists: false,
  }),
  entry(7, {
    ...byViewer,
    action: "setting_changed",
    target_kind: "setting",
    target_id: "registrations.mode",
    target_label: "registrations.mode",
    target_exists: true,
  }),
  entry(8, {
    ...byMona,
    action: "appeal_upheld",
    target_kind: "appeal",
    target_id: storyId("f1", 3),
    target_label: storyId("f1", 3),
    target_exists: true,
  }),
  entry(9, {
    actor_kind: "remote",
    action: "account_moved",
    target_kind: "account",
    target_id: remoteAccountFixture.id,
    target_label: "@carol@remote.example",
    target_exists: true,
  }),
]);

const meta = {
  title: "Screens/Moderation/AuditLog",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/moderation/audit-log", signedIn: true, handlers: [moderatorViewer, entries] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Entries of moderators, the CLI, the server and a remote server, one target deleted since. */
export const Normal: Story = showing("spam.example");

/** Filtered by an action and a kind of target, the filters filled from the address. */
export const Filtered: Story = {
  parameters: { screen: { path: "/moderation/audit-log?action=account_action_issued&target_kind=account" } },
};

/** Nothing has been recorded yet. */
export const Empty: Story = answering(listing("/admin/audit_log", []));

/** The log has not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/audit_log");

/** The server failed to answer the log. */
export const ServerError: Story = failingOn("get", "/admin/audit_log");

/** A member without view_audit is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
