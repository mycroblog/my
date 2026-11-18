// SPDX-License-Identifier: AGPL-3.0-only

// Invites, the entry of the settings at /settings/invites (#3207), and the screens under it, on the fake server: the
// entry as an account that may create invites reads it, an invite in every state among its own; with no invite yet;
// with as many usable invites as the server allows; with more than a page of the list holds; opened by its address by
// an account that may not create invites,
// for each cause the server names and for none; on a server that takes no new account; while it loads, and when it
// could not be read. The menu with the entry and without it, and a one-person server, which has no invites: no
// entry, and the address names no screen. Under it: the form that creates an invite, also for an account that may
// not and for one at the server's limit; and an invite in each state, and an address that names none. What a press
// alone reaches is the parts' own, in the design system's Settings parts: a value shown to copy and what came of
// copying it (its Codes to keep), a form's refusal before its button (its Submit bar), and the confirmation before
// something ends (Dialog).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const invites = "/settings/invites";
const invite = (index: number) => `${invites}/{invite:${String(index)}}`;

const meta = {
  title: "Settings invites",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: invites }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The entry as an account that may create invites reads it: six invites, one in each state and a second usable one. */
export const Entry: Story = {};
/** No invite yet: a sentence in the list's place, and Create invite under it. */
export const NoInvites: Story = { parameters: screen({ at: invites, server: { access: { invites: "none" } } }) };
/** As many usable invites as the server allows: the section says so, with no way to create. */
export const AtCap: Story = { parameters: screen({ at: invites, server: { access: { invites: "at-cap" } } }) };
/** More invites than a page of the list holds: Show more under the first twenty. */
export const Many: Story = { parameters: screen({ at: invites, server: { access: { invites: "many" } } }) };
/** The entry while what it shows loads. */
export const Loading: Story = { parameters: screen({ at: invites, state: "loading" }) };
/** The entry when what it shows could not be read: it says so, with the way to try again. */
export const Failed: Story = { parameters: screen({ at: invites, state: "error" }) };

// ---- The menu --------------------------------------------------------------------------------------------------------

/** The settings' menu of an account that may create invites: Invites, after Apps. */
export const MenuWithInvites: Story = { parameters: screen({ at: "/settings" }) };
/** The menu of an account that may not: no Invites. */
export const MenuWithoutInvites: Story = {
  parameters: screen({ at: "/settings", server: { access: { invites: "not-allowed" } } }),
};

// ---- Opened by its address without being allowed ---------------------------------------------------------------------

/** The server lets only some accounts create invites: the summary says so, and the invites made before do not work. */
export const NotPermitted: Story = {
  parameters: screen({ at: invites, server: { access: { invites: "not-allowed", invitesRefusal: "not_permitted" } } }),
};
/** The account's registration is not approved. */
export const NotApproved: Story = {
  parameters: screen({ at: invites, server: { access: { invites: "not-allowed", invitesRefusal: "not_approved" } } }),
};
/** A moderation action is on the account. */
export const Restricted: Story = {
  parameters: screen({ at: invites, server: { access: { invites: "not-allowed", invitesRefusal: "restricted" } } }),
};
/** The server names no cause. */
export const NotAllowedUnknown: Story = {
  parameters: screen({ at: invites, server: { access: { invites: "not-allowed", invitesRefusal: "unknown" } } }),
};
/** The server takes no new account: nobody creates an invite, and the account's own invites stay as they are. */
export const RegistrationClosed: Story = {
  parameters: screen({ at: invites, server: { registrations: "closed" } }),
};
/** An account that may not create invites and made none. */
export const NotAllowedNoInvites: Story = {
  parameters: screen({ at: invites, server: { registrations: "closed", access: { invites: "none" } } }),
};
/** A one-person server has no invites: the address names no screen, and the menu beside it has no entry. */
export const SingleUser: Story = { parameters: screen({ at: invites, viewer: "hana", server: { singleUser: true } }) };
/** The same for the address of a screen under it. */
export const SingleUserCreate: Story = {
  parameters: screen({ at: `${invites}/new`, viewer: "hana", server: { singleUser: true } }),
};

// ---- Creating an invite ----------------------------------------------------------------------------------------------

/** Creating an invite: the form, at the server's defaults. */
export const Create: Story = { parameters: screen({ at: `${invites}/new` }) };
/** The same address for an account that may not create invites: the summary says why, in the form's place. */
export const CreateNotAllowed: Story = {
  parameters: screen({ at: `${invites}/new`, server: { access: { invites: "not-allowed" } } }),
};
/** The same address with as many usable invites as the server allows: it says so, in the form's place. */
export const CreateAtCap: Story = {
  parameters: screen({ at: `${invites}/new`, server: { access: { invites: "at-cap" } } }),
};

// ---- An invite -------------------------------------------------------------------------------------------------------

/** A usable invite: its link to copy, what it allows, and revoking it. */
export const Usable: Story = { parameters: screen({ at: invite(0) }) };
/** A usable invite that was used once out of five. */
export const UsableUsed: Story = { parameters: screen({ at: invite(1) }) };
/** A revoked invite: its state alone. */
export const Revoked: Story = { parameters: screen({ at: invite(2) }) };
/** An expired invite. */
export const Expired: Story = { parameters: screen({ at: invite(3) }) };
/** An invite used as often as it allows. */
export const UsedUp: Story = { parameters: screen({ at: invite(4) }) };
/** An invite of an account that may not create invites now, which does not work meanwhile: no link, and it can be revoked. */
export const CreatorIneligible: Story = {
  parameters: screen({ at: invite(0), server: { access: { invites: "not-allowed" } } }),
};
/** The oldest of more invites than a page of the list holds: the screen reads the list on until it finds it. */
export const Older: Story = { parameters: screen({ at: invite(25), server: { access: { invites: "many" } } }) };
/** An address that names no invite of the reader's: the screen says so, and leads to the list. */
export const Missing: Story = { parameters: screen({ at: invite(99) }) };
