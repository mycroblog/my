// SPDX-License-Identifier: AGPL-3.0-only

// Security, the entry of the settings at /settings/security (#3205), and the screens under it, on the fake server:
// the entry as an account reads it, with two-factor authentication off (the status summary is the one door to it) and
// on (its row is); for an account without a password, without passkeys, whose password is its only way to sign in
// (it cannot be removed, and its row says why), and alone in its one browser; on a server without login providers,
// and on one that limits sign-in to them (no password to set, no passkey to add); and as the browser comes back from
// a login provider, with each thing that may have happened there in the fragment of the address. Under it: the
// password's screen, for an account with one, without one, and back from a provider's confirmation; adding a
// passkey; and two-factor authentication, off (the setup) and on. What a press alone reaches is the parts' own, in the design system's Settings parts: the confirmation that
// asks to authenticate again by each proof, the recovery codes shown once, and a confirmation before something ends
// (its List management).

import { fakeExternalConfirmation } from "@my/e2e/fake";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const security = "/settings/security";
const password = `${security}/password`;
const passkey = `${security}/passkey`;
const twoFactor = `${security}/two-factor`;

const meta = {
  title: "Settings security",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: security }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The entry as a typical account reads it: two-factor authentication is off, and the summary says so. */
export const Entry: Story = {};
/** Two-factor authentication on: the summary is gone, and its row in Sign-in is the door to it. */
export const TwoFactorOn: Story = { parameters: screen({ at: security, server: { access: { twoFactor: true } } }) };
/** An account without a password, which signs in with its passkeys and its linked account. */
export const NoPassword: Story = { parameters: screen({ at: security, server: { access: { password: false } } }) };
/** An account without passkeys: the sentence and Add in the list's place. */
export const NoPasskeys: Story = { parameters: screen({ at: security, server: { access: { passkeys: 0 } } }) };
/** The password is the only way to sign in: it cannot be removed, and its row says why, with no button. */
export const PasswordOnlyWay: Story = {
  parameters: screen({ at: security, server: { access: { passkeys: 0, linked: "none" } } }),
};
/** A server that limits sign-in to login providers, for an account without passkeys: Passkeys is absent. */
export const PasskeysNotOffered: Story = {
  parameters: screen({ at: security, server: { access: { providersOnly: true, passkeys: 0, twoFactor: true } } }),
};
/** The same server for an account that holds passkeys: it may still remove them, and add none. */
export const SignInLimited: Story = {
  parameters: screen({ at: security, server: { access: { providersOnly: true } } }),
};
/** A server without login providers: Linked accounts is absent. */
export const NoProviders: Story = {
  parameters: screen({ at: security, server: { access: { linked: "no-providers" } } }),
};
/** No account linked yet: each provider that allows it offers Link. */
export const NothingLinked: Story = { parameters: screen({ at: security, server: { access: { linked: "none" } } }) };
/** The one browser signed in: no other to sign out. */
export const OneBrowser: Story = { parameters: screen({ at: security, server: { access: { sessions: "alone" } } }) };
/** The entry while what it shows loads. */
export const Loading: Story = { parameters: screen({ at: security, state: "loading" }) };
/** The entry when what it shows could not be read: it says so, with the way to try again. */
export const Failed: Story = { parameters: screen({ at: security, state: "error" }) };

// ---- Back from a login provider ---------------------------------------------------------------------------------------

/** Back from GitHub with the account linked. */
export const ProviderLinked: Story = { parameters: screen({ at: `${security}#linked={provider:github}` }) };
/** Back from a provider that refused the account. */
export const ProviderDenied: Story = {
  parameters: screen({
    at: `${security}#external_error=denied&provider={provider:oidc}`,
    server: { access: { linked: "none" } },
  }),
};
/** Back from a provider whose account another user of the server links. */
export const ProviderLinkedElsewhere: Story = {
  parameters: screen({ at: `${security}#external_error=linked_elsewhere&provider={provider:oidc}` }),
};
/** Back from a confirmation of a link at GitHub: the section says to press Link again, which then asks nothing. */
export const ProviderConfirmed: Story = {
  parameters: screen({
    at: `${security}#external_confirmation=${fakeExternalConfirmation}&operation=link&provider={provider:github}`,
  }),
};
/** Back from a confirmation where the reader signed in with another account than the linked one. */
export const ProviderMismatch: Story = {
  parameters: screen({ at: `${security}#external_error=mismatch&operation=link&provider={provider:github}` }),
};
/** Back from GitHub, which confirmed removing the password: Sign-in says to press Remove again. */
export const ProviderConfirmedRemoval: Story = {
  parameters: screen({
    at: `${security}#external_confirmation=${fakeExternalConfirmation}&operation=password_removal&provider={provider:github}`,
  }),
};
/** The address the server sends the browser back to by default leads to Security, with what happened. */
export const ProviderDefaultReturn: Story = {
  parameters: screen({ at: "/settings/linked-accounts#linked={provider:github}" }),
};

// ---- The password ---------------------------------------------------------------------------------------------------

/** The password of an account that has one: the new password, and right after it the button that changes it. */
export const Password: Story = { parameters: screen({ at: password }) };
/** An account without a password sets one. */
export const PasswordUnset: Story = { parameters: screen({ at: password, server: { access: { password: false } } }) };
/** A server that limits sign-in to login providers: no password to set or change, and the screen says so. */
export const PasswordLimited: Story = {
  parameters: screen({ at: password, server: { access: { providersOnly: true } } }),
};
/** Back from GitHub, which confirmed a change of the password: the screen asks to enter it again. */
export const PasswordConfirmedElsewhere: Story = {
  parameters: screen({
    at: `${password}#external_confirmation=${fakeExternalConfirmation}&operation=password_change&provider={provider:github}`,
  }),
};

// ---- A passkey, and two-factor authentication -----------------------------------------------------------------------

/** Adding a passkey: its name, then the browser's own dialog. */
export const PasskeyAdd: Story = { parameters: screen({ at: passkey }) };
/** Turning two-factor authentication on: the key for the authenticator app, then its code. */
export const TwoFactorSetup: Story = { parameters: screen({ at: twoFactor }) };
/** Two-factor authentication on: the recovery codes left, and the ways to make them again and to turn it off. */
export const TwoFactorManage: Story = {
  parameters: screen({ at: twoFactor, server: { access: { twoFactor: true } } }),
};
