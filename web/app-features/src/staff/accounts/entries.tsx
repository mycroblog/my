// SPDX-License-Identifier: AGPL-3.0-only

// The entries of accounts and sanctions among the staff's screens (entries.tsx; FR-703–FR-721, FR-011, ADR-0058): in
// the queues, the appeals of actions and the registrations that wait, each counted, and the automated reviews' records;
// under Accounts, the search of accounts, the blocks on sign-ups and the invites. A one-person server offers neither
// registrations, sign-up blocks nor invites (ADR-0152).

import {
  type AppData,
  loadHookDecisions,
  loadModeratedAccountSearch,
  loadModeratedAppeals,
  loadRegistrations,
  loadSignupBlocks,
  loadStaffInvites,
} from "@my/app-data";
import { loadStaffEntry, type StaffEntry } from "../entries";
import { appealStateOf, AppealQueueScreen } from "./appeals";
import { decisionQueryOf, HookDecisionsScreen } from "./hooks";
import { InvitesScreen } from "./invites";
import { RegistrationsScreen } from "./registrations";
import { AccountSearchScreen, searchQueryOf } from "./search";
import { blockQueryOf, SignupBlocksScreen } from "./signup-blocks";
import { cursorOf } from "./parts";
import { useStaffStrings } from "../parts";

/** The groups these entries add to the root of Moderation, after the queues. */
export const accountGroups = [
  { id: "accounts", area: "moderation", heading: "staff_accounts.group.accounts" },
] as const;

/** Whether the server offers what a one-person server does not: registrations, sign-up blocks and invites. */
const many = (facts: { readonly singleUser: boolean | undefined }) => facts.singleUser === false;

/** The entries of accounts and sanctions, in the root's order within their groups. */
export const accountEntries: readonly StaffEntry[] = [
  {
    // The appeals of actions (FR-707): those pending first, which the badge of Moderation counts with the rest.
    path: "appeals",
    area: "moderation",
    group: "queues",
    permission: "manage_accounts",
    icon: "appeal",
    name: "staff_accounts.entry.appeals",
    hint: "staff_accounts.entry.appeals.hint",
    count: { of: (facts) => facts.pendingAppeals, words: "staff_accounts.entry.appeals.count" },
    Screen: AppealQueueScreen,
    load: (data, search) => loadModeratedAppeals(data, appealStateOf(search), cursorOf(search)),
  },
  {
    // The registrations that wait for approval (FR-721), counted, which a one-person server never has.
    path: "registrations",
    area: "moderation",
    group: "queues",
    permission: "manage_accounts",
    icon: "signUp",
    name: "staff_accounts.entry.registrations",
    hint: "staff_accounts.entry.registrations.hint",
    count: { of: (facts) => facts.pendingRegistrations, words: "staff_accounts.entry.registrations.count" },
    shown: many,
    Screen: RegistrationsScreen,
    load: (data, search) => loadRegistrations(data, cursorOf(search)),
  },
  {
    // What the decision hooks stopped, would have stopped, or could not check (ADR-0147), and their appeals.
    path: "hook-decisions",
    area: "moderation",
    group: "queues",
    permission: "manage_reports",
    icon: "reviewFailing",
    name: "staff_accounts.entry.hook_decisions",
    hint: "staff_accounts.entry.hook_decisions.hint",
    Screen: HookDecisionsScreen,
    load: (data, search) => loadHookDecisions(data, decisionQueryOf(search)),
  },
  {
    // The accounts (FR-714), each with its review, its actions and its management.
    path: "accounts",
    area: "moderation",
    group: "accounts",
    permission: "manage_accounts",
    icon: "members",
    name: "staff_accounts.entry.accounts",
    hint: "staff_accounts.entry.accounts.hint",
    Screen: AccountSearchScreen,
    load: (data, search) => loadModeratedAccountSearch(data, searchQueryOf(search)),
  },
  {
    // The blocks on sign-ups (FR-719).
    path: "signup-blocks",
    area: "moderation",
    group: "accounts",
    permission: "manage_accounts",
    icon: "block",
    name: "staff_accounts.entry.signup_blocks",
    hint: "staff_accounts.entry.signup_blocks.hint",
    shown: many,
    Screen: SignupBlocksScreen,
    load: (data, search) => loadSignupBlocks(data, blockQueryOf(search)),
  },
  {
    // The invites of the staff and of the members, with who created each (FR-011).
    path: "invites",
    area: "moderation",
    group: "accounts",
    permission: "manage_invites",
    icon: "invite",
    name: "staff_accounts.entry.invites",
    hint: "staff_accounts.entry.invites.hint",
    shown: many,
    Screen: InvitesScreen,
    load: (data, search) => loadStaffInvites(data, cursorOf(search)),
  },
];

/**
 * Reads what a screen under the entry of moderation at `path` shows (an account, an appeal, a block), with `load`,
 * once the reader's permissions open the entry, for its route's loader.
 */
export function loadUnderEntry(data: AppData, path: string, load: (data: AppData) => Promise<void>): Promise<void> {
  return loadStaffEntry(data, "moderation", path, {}, load);
}

/** The titles of the screens under these entries that name nothing they read: the forms, and the appeals of refused posts. */
const underTitles = {
  "new-block": "staff_accounts.blocks.add",
  "import-blocks": "staff_accounts.blocks.import",
  "new-invite": "staff_accounts.invites.create",
  "hook-appeals": "staff_accounts.hook_appeals.title",
} as const;

/** The title of the screen `which` under these entries. */
export function useUnderTitle(which: keyof typeof underTitles): string {
  return useStaffStrings().t(underTitles[which]);
}
