// SPDX-License-Identifier: AGPL-3.0-only

// Leaving an account in the settings' entry Account and data (FR-008, FR-009, ADR-0089 §3, §8, §11–§17): the entry's
// last two parts and the three screens behind them, on the fake server as yuki reads them.
//
// - The entry: the door to Move account, then, last, the doors to Deactivate account and Delete account; and for an
//   account that has moved, whose door says where.
// - Move account: the form, empty, under what a move does and what to do first on the other account; back from a
//   login provider's confirmation, the account named before leaving in the field again; an account that has moved,
//   with the way to cancel; while it loads, and when it could not be read. What follows a press is shot by a script
//   that types and presses: the field filled; an address the fake finds no account at (nobody@nowhere.example), and
//   the reader's own (yuki), each refused under the field; the confirmation open (sora@photos.example), and back
//   from the provider the plain question of the act; and, on a fake that refuses every move once it is confirmed,
//   each refusal: the account named does not list this one among its aliases, moved itself, cannot be moved to, or
//   is one this account moved to before, under the field, and a move too soon after the last, which takes the act's
//   place. An account that moved 12 days ago, whose act's place says from when it can move again. On the moved
//   account, the question before the move is cancelled, and once it is, the screen again with the act's place saying
//   from when it can move again.
// - Deactivate account: what it does and until when it can be undone, the door to Export, and the act; each state in
//   which the server refuses, said in the act's place with what to do first; while it loads, and when it could not
//   be read. The confirmation open, and back from the provider the plain question of the act, are shot by a script
//   that presses.
// - Delete account: what it does, the door to Export, and the act; for the server's only administrator, the refusal
//   in the act's place, known as the screen opens or, where what deactivating would do says another thing first,
//   once a script pressed and confirmed; while it loads, and when it could not be read.
//
// A confirmation holds with yuki's password, with a passkey, or with the fake's linked provider.

import { confirmationKeptKey } from "@my/app-features";
import { fakeExternalConfirmation, type FakeOptions } from "@my/e2e/fake";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const entry = "/settings/account";
const moveAt = "/settings/account/move";
const deactivateAt = "/settings/account/deactivate";
const deleteAt = "/settings/account/delete";

type Settings = NonNullable<FakeOptions["settings"]>;

/** The person the reader's account moved to, in the stories of a moved account. */
const movedTo = "sora@photos.example";

/** A story of Move account on a fake that refuses every move once it is confirmed, for `reason`. */
const moveRefused = (reason: NonNullable<Settings["moveRefusal"]>) =>
  screen({ at: moveAt, server: { settings: { moveRefusal: reason } } });

/** A story of Deactivate account for an account the server would not deactivate, for `reason`. */
const deactivationRefused = (reason: NonNullable<Settings["deactivationRefusal"]>) =>
  screen({ at: deactivateAt, server: { settings: { deactivationRefusal: reason } } });

/** What a login provider's return leaves in the address once the reader confirmed `operation` there. */
const confirmedAt = (at: string, operation: string) =>
  `${at}#external_confirmation=${fakeExternalConfirmation}&operation=${operation}&provider={provider:github}`;

const meta = {
  title: "Settings account leave",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: entry }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * /settings/account as the fake starts: after the doors of the reader's data and of Aliases, the door to Move
 * account, and last, apart, the doors to Deactivate account and Delete account.
 */
export const Entry: Story = {};
/** The entry of an account that has moved: the door to Move account says where. */
export const EntryMoved: Story = { parameters: screen({ at: entry, server: { settings: { movedTo } } }) };

/**
 * /settings/account/move before a move: what a move does, what to do first on the other account with this account's
 * handle to copy, and the field that names the account to move to over the button that moves.
 */
export const Move: Story = { parameters: screen({ at: moveAt }) };
/**
 * Back from GitHub, which confirmed a move: the account named before leaving is in the field again, and the form says
 * to move again, which then asks only whether to.
 */
export const MoveConfirmedElsewhere: Story = {
  parameters: screen({ at: confirmedAt(moveAt, "account_move") }),
  // What the tab kept of the form as the reader left for the provider.
  beforeEach: () => {
    const key = confirmationKeptKey("account_move");
    sessionStorage.setItem(key, movedTo);
    return () => {
      sessionStorage.removeItem(key);
    };
  },
};
/** A fake that refuses the move because the account named does not list this one among its aliases. */
export const MoveAliasMissing: Story = { parameters: moveRefused("alias_missing") };
/** A fake that refuses the move because the account named moved itself. */
export const MoveTargetMoved: Story = { parameters: moveRefused("target_moved") };
/** A fake that refuses the move because the account named cannot be moved to. */
export const MoveTargetUnavailable: Story = { parameters: moveRefused("target_unavailable") };
/** A fake that refuses the move because this account moved to the account named before. */
export const MoveRepeated: Story = { parameters: moveRefused("repeated") };
/**
 * A fake that refuses the move because the account moved in the last 30 days, since the screen read it: once pressed,
 * the act's place says from when it can move again, with no field and no button.
 */
export const MoveTooSoon: Story = { parameters: moveRefused("too_soon") };
/**
 * An account that moved 12 days ago, and cancelled that move: the act's place says when it last moved and from when
 * it can move again, with no field and no button.
 */
export const MoveLater: Story = { parameters: screen({ at: moveAt, server: { settings: { lastMoved: "-12d" } } }) };
/** An account that has moved: where, what it can still do, and, in a section of its own, cancelling the move. */
export const Moved: Story = { parameters: screen({ at: moveAt, server: { settings: { movedTo } } }) };
/** Move account while the account is on its way. */
export const MoveLoading: Story = { parameters: screen({ at: moveAt, state: "loading" }) };
/** Move account when the account could not be read. */
export const MoveFailed: Story = { parameters: screen({ at: moveAt, state: "error" }) };

/**
 * /settings/account/deactivate: what deactivating does, until when signing in again brings the account back, the door
 * to Export, and last the button that deactivates.
 */
export const Deactivate: Story = { parameters: screen({ at: deactivateAt }) };
/** Back from GitHub, which confirmed the deactivation: the act says to press again, which then asks only whether to. */
export const DeactivateConfirmedElsewhere: Story = {
  parameters: screen({ at: confirmedAt(deactivateAt, "account_deactivation") }),
};
/** An account that has moved cannot be deactivated: the summary says so and leads to cancelling the move. */
export const DeactivateMoved: Story = { parameters: screen({ at: deactivateAt, server: { settings: { movedTo } } }) };
/** The only administrator who can sign in cannot deactivate their account; the app has no screen of roles to lead to. */
export const DeactivateLastAdministrator: Story = { parameters: deactivationRefused("last_administrator") };
/** A supporter plan that goes on is cancelled first; the app has no screen of the plan to lead to. */
export const DeactivatePaidSubscription: Story = { parameters: deactivationRefused("paid_subscription") };
/** An account reactivated in the last day: the summary says from when it can be deactivated again. */
export const DeactivateRecentlyReactivated: Story = { parameters: deactivationRefused("recently_reactivated") };
/** A moderation action stops the account's sign-in: the summary leads to Account status. */
export const DeactivateLoginRestricted: Story = { parameters: deactivationRefused("login_restricted") };
/** The account's deletion is scheduled already: the summary leads to Account status. */
export const DeactivateDeletionScheduled: Story = { parameters: deactivationRefused("deletion_scheduled") };
/** The account's registration waits for a moderator's approval. */
export const DeactivateRegistrationPending: Story = { parameters: deactivationRefused("registration_pending") };
/** Deactivate account while what deactivating would do is on its way. */
export const DeactivateLoading: Story = { parameters: screen({ at: deactivateAt, state: "loading" }) };
/** Deactivate account when what deactivating would do could not be read. */
export const DeactivateFailed: Story = { parameters: screen({ at: deactivateAt, state: "error" }) };

/** /settings/account/delete: what deleting does, the door to Export, and last the button that deletes. */
export const Delete: Story = { parameters: screen({ at: deleteAt }) };
/** Back from GitHub, which confirmed the deletion: the act says to press again, which then asks only whether to. */
export const DeleteConfirmedElsewhere: Story = {
  parameters: screen({ at: confirmedAt(deleteAt, "account_deletion") }),
};
/** The only administrator who can sign in, as what deactivating would do says: the act's place says so. */
export const DeleteLastAdministrator: Story = {
  parameters: screen({ at: deleteAt, server: { settings: { deactivationRefusal: "last_administrator" } } }),
};
/**
 * A fake whose only administrator the reader is while what deactivating would do says nothing of it: the deletion is
 * refused once it is confirmed, and the summary then takes the act's place.
 */
export const DeleteLastAdministratorPressed: Story = {
  parameters: screen({ at: deleteAt, server: { settings: { deletionRefusal: "last_administrator" } } }),
};
/** Delete account while the account is on its way. */
export const DeleteLoading: Story = { parameters: screen({ at: deleteAt, state: "loading" }) };
/** Delete account when the account could not be read. */
export const DeleteFailed: Story = { parameters: screen({ at: deleteAt, state: "error" }) };
