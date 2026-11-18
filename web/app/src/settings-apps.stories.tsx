// SPDX-License-Identifier: AGPL-3.0-only

// Apps, the entry of the settings at /settings/apps (#3206), and the screens under it, on the fake server: the entry
// as an account reads it, the server's own web client among the apps it authorized and three apps of its own, one of
// them disabled; with no app in either list; on a server whose administrators turned registration off, which the list
// of the reader's apps says; while it loads, and when it could not be read. Under it:
// an authorized app of each kind of registration, the server's own web client, whose revocation signs every browser
// out, and an address that names none; the form that registers an app; an app of the reader's, confidential, public
// and disabled, and an address that names none; and the form that issues a token, also for a disabled app. What a
// press alone reaches is the parts' own, in the design system's Settings parts: a value shown once to copy (its Codes
// to keep and Authenticator setup), a refused field (Fields), and the confirmation before something ends (Dialog).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const apps = "/settings/apps";
const authorized = (index: number) => `${apps}/authorized/{app:authorized:${String(index)}}`;
const own = (index: number) => `${apps}/{app:own:${String(index)}}`;

const meta = {
  title: "Settings apps",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: apps }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The entry as a typical account reads it: four authorized apps, and three of its own. */
export const Entry: Story = {};
/** No app in either list: a sentence in each list's place, and Register an app under the second. */
export const NoApps: Story = {
  parameters: screen({ at: apps, server: { access: { authorizedApps: "none", oauthApps: "none" } } }),
};
/** A server that does not let users register apps: the section says so, with no way to register. */
export const RegistrationOff: Story = {
  parameters: screen({ at: apps, server: { access: { oauthApps: "disabled" } } }),
};
/** The entry while what it shows loads. */
export const Loading: Story = { parameters: screen({ at: apps, state: "loading" }) };
/** The entry when what it shows could not be read: it says so, with the way to try again. */
export const Failed: Story = { parameters: screen({ at: apps, state: "error" }) };

// ---- An authorized app -----------------------------------------------------------------------------------------------

/** An app that registered itself: not verified, with permissions of every weight. */
export const AuthorizedSelfRegistered: Story = { parameters: screen({ at: authorized(1) }) };
/** An app known by its metadata document, whose host the server verified: it only reads. */
export const AuthorizedByDocument: Story = { parameters: screen({ at: authorized(2) }) };
/** An app a user of the server registered, which has not used the account yet. */
export const AuthorizedByUser: Story = { parameters: screen({ at: authorized(3) }) };
/** The server's own web client: revoking it signs the account out of every browser. */
export const AuthorizedWebClient: Story = { parameters: screen({ at: authorized(0) }) };
/** An address that names no authorized app: the screen says so, and leads to the list. */
export const AuthorizedMissing: Story = { parameters: screen({ at: authorized(9) }) };

// ---- An app of the reader's own ----------------------------------------------------------------------------------------

/** Registering an app: the empty form. */
export const Register: Story = { parameters: screen({ at: `${apps}/new` }) };
/** The same screen on a server that does not let users register apps: it says so in the form's place. */
export const RegisterOff: Story = {
  parameters: screen({ at: `${apps}/new`, server: { access: { oauthApps: "disabled" } } }),
};
/** A confidential app: its client ID, a new secret, a token, its settings, and deleting it. */
export const OwnConfidential: Story = { parameters: screen({ at: own(0) }) };
/** A public app: no secret to make. */
export const OwnPublic: Story = { parameters: screen({ at: own(1) }) };
/** An app the administrators disabled: the summary says so. */
export const OwnDisabled: Story = { parameters: screen({ at: own(2) }) };
/** An address that names no app of the reader's: the screen says so, and leads to the list. */
export const OwnMissing: Story = { parameters: screen({ at: own(9) }) };
/** Issuing a token of an app for the reader's own account: the scopes it carries. */
export const Token: Story = { parameters: screen({ at: `${own(0)}/token` }) };
/** No token for an app the administrators disabled. */
export const TokenDisabled: Story = { parameters: screen({ at: `${own(2)}/token` }) };
