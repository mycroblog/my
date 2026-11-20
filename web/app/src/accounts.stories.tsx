// SPDX-License-Identifier: AGPL-3.0-only

// The moderation of accounts (ADR-0058 §2; FR-703, FR-707, FR-708, FR-713, FR-714, FR-719, FR-721, FR-011, FR-931), on
// the fake server (app.samples.tsx), whose moderation (web/e2e/src/fake/accounts.ts) holds a limited spam account with
// an expired warning, kai under no posting with an appeal that waits, a banned seller of followers whose appeal waits,
// mio's lifted suspension whose appeal was overturned, layla's upheld warning, a remote photographer's media always
// sensitive, three registrations that wait, the automated reviews' decisions and two appeals of refused posts, five
// sign-up blocks and seven invites, read by the scenario's moderator (marco) unless a story names another reader.
// The roots count the appeals and registrations that wait, and a one-person server offers neither registrations,
// sign-up blocks nor invites. Each screen in its states: its list, filtered, empty, loading and failing; each thing,
// missing; and, for a reader whose permissions do not open it, the address that names nothing open to them. What
// follows a press (an action taken, a ban asked, a showing of the email and IPs, a decision) is the route tests'.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const meta = {
  title: "Moderation/Accounts",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/moderation", viewer: "marco" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `at` as the scenario's moderator reads it, unless `more` names another reader. */
const at = (path: string, more: Omit<Fake, "at"> = {}) => screen({ at: path, viewer: "marco", ...more });

/** Moderation for the administrator: the appeals and the registrations that wait counted, and the accounts' group. */
export const Root: Story = { parameters: at("/moderation", { viewer: "hana" }) };
/** Moderation of a one-person server: no registrations, sign-up blocks or invites. */
export const RootSingleUser: Story = {
  parameters: at("/moderation", { viewer: "hana", server: { singleUser: true } }),
};

/** The accounts, newest first: each its handle, its server, when it was created and its state. */
export const Accounts: Story = { parameters: at("/moderation/accounts") };
/** The accounts' second page, two a page. */
export const AccountsSecondPage: Story = {
  parameters: at("/moderation/accounts", { server: { staffPage: 2 }, next: true }),
};
/** The accounts of other servers. */
export const AccountsRemote: Story = { parameters: at("/moderation/accounts?origin=remote") };
/** The accounts that signed up or signed in from one IP address. */
export const AccountsByIp: Story = { parameters: at("/moderation/accounts?by=ip&q=203.0.113.24") };
/** A search that finds nothing. */
export const AccountsNoMatch: Story = { parameters: at("/moderation/accounts?q=nobodyhere") };
export const AccountsLoading: Story = { parameters: at("/moderation/accounts", { state: "loading" }) };
export const AccountsError: Story = { parameters: at("/moderation/accounts", { state: "error" }) };
/** The accounts' address, for a reader whose permissions do not open it. */
export const AccountsNotOpen: Story = { parameters: at("/moderation/accounts", { viewer: "yuki" }) };

/** A limited spam account's Review: its facts, the form of a new action, its actions and its reports. */
export const Account: Story = { parameters: at("/moderation/accounts/{account:quickdeals}") };
/**
 * kai, reported more often than the account's details hold: the newest 20 reports and Show more, which reads the rest.
 */
export const AccountManyReports: Story = {
  parameters: at("/moderation/accounts/{account:kai}", { server: { reports: "crowded", moderation: "crowded" } }),
};
/** The same, opened from a report of its posts: the actions on posts offered, naming the report's posts. */
export const AccountFromReport: Story = {
  parameters: at("/moderation/accounts/{account:quickdeals}?report={report:deals}"),
};
/** A remote account's Review: its actions are limit, suspend and media always sensitive. */
export const AccountRemote: Story = { parameters: at("/moderation/accounts/{account:sora@photos.example}") };
/** A banned account's Review, its ban among its actions. */
export const AccountBanned: Story = { parameters: at("/moderation/accounts/{moderated:account:cheapfollowers}") };
/** An applicant's account, whose registration waits, signed up with an invite. */
export const AccountPending: Story = {
  parameters: at("/moderation/accounts/{moderated:account:breadandmountainsandlongwalks}"),
};
export const AccountLoading: Story = {
  parameters: at("/moderation/accounts/{account:quickdeals}", { state: "loading" }),
};
export const AccountError: Story = { parameters: at("/moderation/accounts/{account:quickdeals}", { state: "error" }) };
/** An address that names no account. */
export const AccountMissing: Story = { parameters: at("/moderation/accounts/none") };

/** Every action on the spam account, the lifted and the expired among them. */
export const AccountActions: Story = { parameters: at("/moderation/accounts/{account:quickdeals}/actions") };
/** Its actions in effect alone. */
export const AccountActionsActive: Story = {
  parameters: at("/moderation/accounts/{account:quickdeals}/actions?active=true"),
};
/** An account no action was taken on. */
export const AccountActionsNone: Story = { parameters: at("/moderation/accounts/{account:yuki}/actions") };

/** A limit in effect: all it holds, and Lift with its reason. */
export const Action: Story = {
  parameters: at("/moderation/accounts/{account:quickdeals}/actions/{moderated:action:quickdeals:limit}"),
};
/** A suspension lifted when its appeal was overturned. */
export const ActionLifted: Story = {
  parameters: at("/moderation/accounts/{account:mio}/actions/{moderated:action:mio}"),
};
/** A ban: permanent, never lifted; only an appeal can overturn it, whose way its page offers. */
export const ActionBan: Story = {
  parameters: at("/moderation/accounts/{moderated:account:cheapfollowers}/actions/{moderated:action:ban}"),
};
/** An address that names no action of the account. */
export const ActionMissing: Story = { parameters: at("/moderation/accounts/{account:quickdeals}/actions/none") };

/** A local account's Management for the administrator: its email and IPs kept until shown, and sign-in recovery. */
export const Manage: Story = { parameters: at("/moderation/accounts/{account:kai}/manage", { viewer: "hana" }) };
/** An account whose email bounced: the suppression, and Send email again. */
export const ManageSuppressed: Story = {
  parameters: at("/moderation/accounts/{account:sam}/manage", { viewer: "hana" }),
};
/** A moderator's Management of a local account: no sign-in recovery, which needs the administration's permission. */
export const ManageModerator: Story = { parameters: at("/moderation/accounts/{account:kai}/manage") };
/** A remote account's Management: fetching its posts again. */
export const ManageRemote: Story = {
  parameters: at("/moderation/accounts/{account:sora@photos.example}/manage", { viewer: "hana" }),
};

/** The appeals that wait: kai's of no posting, and the ban's. */
export const Appeals: Story = { parameters: at("/moderation/appeals") };
/** Every appeal, on its second page, two a page. */
export const AppealsSecondPage: Story = {
  parameters: at("/moderation/appeals?state=any", { server: { staffPage: 2 }, next: true }),
};
/** Every appeal, decided ones among them. */
export const AppealsAll: Story = { parameters: at("/moderation/appeals?state=any") };
/** No appeal waits. */
export const AppealsEmpty: Story = { parameters: at("/moderation/appeals", { server: { moderation: "none" } }) };
export const AppealsLoading: Story = { parameters: at("/moderation/appeals", { state: "loading" }) };
export const AppealsError: Story = { parameters: at("/moderation/appeals", { state: "error" }) };
/** An appeal that waits: what kai wrote, the action, and the decision. */
export const Appeal: Story = { parameters: at("/moderation/appeals/{moderated:appeal:kai}") };
/** The ban's appeal: overturning it cancels the deletion, as its confirmation says. */
export const AppealOfBan: Story = { parameters: at("/moderation/appeals/{moderated:appeal:ban}") };
/** An overturned appeal, with who decided and the reason given. */
export const AppealOverturned: Story = { parameters: at("/moderation/appeals/{moderated:appeal:mio}") };
/** An address that names no appeal. */
export const AppealMissing: Story = { parameters: at("/moderation/appeals/none") };

/** The registrations that wait: their emails, an unconfirmed one, why they want to join, and a long handle. */
export const Registrations: Story = { parameters: at("/moderation/registrations") };
/** The registrations' second page, two a page. */
export const RegistrationsSecondPage: Story = {
  parameters: at("/moderation/registrations", { server: { staffPage: 2 }, next: true }),
};
/** No registration waits. */
export const RegistrationsEmpty: Story = {
  parameters: at("/moderation/registrations", { server: { moderation: "none" } }),
};
export const RegistrationsLoading: Story = { parameters: at("/moderation/registrations", { state: "loading" }) };
export const RegistrationsError: Story = { parameters: at("/moderation/registrations", { state: "error" }) };
/** The registrations' address on a one-person server, which has none: it names no screen. */
export const RegistrationsSingleUser: Story = {
  parameters: at("/moderation/registrations", { viewer: "hana", server: { singleUser: true } }),
};

/** The automated reviews: each hook's decisions of the week, then what they stopped, flagged or could not check. */
export const HookDecisions: Story = { parameters: at("/moderation/hook-decisions") };
/** The hooks' decisions on their second page, two a page. */
export const HookDecisionsSecondPage: Story = {
  parameters: at("/moderation/hook-decisions", { server: { staffPage: 2 }, next: true }),
};
/** The decisions of one hook. */
export const HookDecisionsOfHook: Story = { parameters: at("/moderation/hook-decisions?hook=inbound.status") };
/** No decision. */
export const HookDecisionsEmpty: Story = {
  parameters: at("/moderation/hook-decisions", { server: { moderation: "none" } }),
};
export const HookDecisionsLoading: Story = { parameters: at("/moderation/hook-decisions", { state: "loading" }) };
/** A drop that can be cancelled until a date. */
export const HookDecision: Story = { parameters: at("/moderation/hook-decisions/{moderated:decision:drop}") };
/** A post refused, which its author's appeal reconsiders. */
export const HookDecisionRefused: Story = { parameters: at("/moderation/hook-decisions/{moderated:decision:reject}") };
/** A decision a moderator cancelled. */
export const HookDecisionCancelled: Story = {
  parameters: at("/moderation/hook-decisions/{moderated:decision:overridden}"),
};
/** The appeals of refused posts that wait. */
export const HookAppeals: Story = { parameters: at("/moderation/hook-decisions/appeals") };
/** Every appeal of the hooks' decisions, on their second page, one a page. */
export const HookAppealsSecondPage: Story = {
  parameters: at("/moderation/hook-decisions/appeals?state=any", { server: { staffPage: 1 }, next: true }),
};
/** An appeal of a refused post: the post, the author's comment, and the decision. */
export const HookAppeal: Story = { parameters: at("/moderation/hook-decisions/appeals/{moderated:hook-appeal:kai}") };
/** An upheld appeal of a refused post. */
export const HookAppealUpheld: Story = {
  parameters: at("/moderation/hook-decisions/appeals/{moderated:hook-appeal:mio}"),
};

/** The sign-up blocks in effect. */
export const SignupBlocks: Story = { parameters: at("/moderation/signup-blocks") };
/** Every sign-up block, the lifted and the expired among them. */
export const SignupBlocksAll: Story = { parameters: at("/moderation/signup-blocks?active=all") };
/** Every sign-up block, on its second page, two a page. */
export const SignupBlocksSecondPage: Story = {
  parameters: at("/moderation/signup-blocks?active=all", { server: { staffPage: 2 }, next: true }),
};
/** No sign-up block. */
export const SignupBlocksEmpty: Story = {
  parameters: at("/moderation/signup-blocks", { server: { moderation: "none" } }),
};
export const SignupBlocksLoading: Story = { parameters: at("/moderation/signup-blocks", { state: "loading" }) };
/** The form of a new block. */
export const SignupBlockNew: Story = { parameters: at("/moderation/signup-blocks/new") };
/** The form of blocks from a list. */
export const SignupBlockImport: Story = { parameters: at("/moderation/signup-blocks/import") };
/** An IP range held for approval until a date, with its comment, and Lift. */
export const SignupBlock: Story = { parameters: at("/moderation/signup-blocks/{moderated:block:range}") };
/** A block of an email address made from a ban. */
export const SignupBlockFromBan: Story = { parameters: at("/moderation/signup-blocks/{moderated:block:ban}") };
/** An address that names no block. */
export const SignupBlockMissing: Story = { parameters: at("/moderation/signup-blocks/none") };

/** Every invite for the administrator: who created each, how many signed up, when it ends, and its state. */
export const Invites: Story = { parameters: at("/moderation/invites", { viewer: "hana" }) };
/** The invites' second page, two a page. */
export const InvitesSecondPage: Story = {
  parameters: at("/moderation/invites", { viewer: "hana", server: { staffPage: 2 }, next: true }),
};
/** No invite. */
export const InvitesEmpty: Story = {
  parameters: at("/moderation/invites", { viewer: "hana", server: { moderation: "none" } }),
};
export const InvitesLoading: Story = { parameters: at("/moderation/invites", { viewer: "hana", state: "loading" }) };
/** The invites' address for a reader whose permissions do not open it. */
export const InvitesNotOpen: Story = { parameters: at("/moderation/invites", { viewer: "yuki" }) };
/** The form of a new invite. */
export const InviteNew: Story = { parameters: at("/moderation/invites/new", { viewer: "hana" }) };

/** A report of a voice chat room still open: End the room, for everyone. */
export const ReportOpenRoom: Story = { parameters: at("/moderation/reports/{report:live}") };
/** A report of an event: Cancel the event beside the way to its page, and Act on this account. */
export const ReportEvent: Story = { parameters: at("/moderation/reports/{report:event}") };
