// SPDX-License-Identifier: AGPL-3.0-only

// Invites, the settings' entry of an account's own invites, and the screens under it, for their tests: each drawn
// alone on a fake server (./settings.tsx), the invites as that server holds them, and the problem a server refuses
// a new invite with. What a test reads of a settings screen is in ./security.ts.

import type { OwnInvite } from "@my/app-data";
import { createFakeApi, fakeOrigin, type Failure, type FakeApi, type FakeOptions } from "@my/e2e/fake";
import { screen, within } from "@testing-library/react";
import { InviteCreate } from "../settings/invites/create";
import { OwnInviteScreen } from "../settings/invites/invite";
import { SettingsEntryScreen } from "../settings/screen";
import { atRest, heldBy } from "./security";
import { settingsOn, type Access, type Given } from "./settings";

/** What every screen of the settings says while it reads, and when it could not. */
export const loading = "Loading your settings…";
export const notLoaded = /^Your settings could not be loaded\.\s*Try again$/;

/** The sentence about a link to send, under "Invite link" and under "The invite is created". */
export const sendThis = (code: string) =>
  `Send this link to the person you invite. They can also enter the invite code ${code} on the sign-up page.`;

/** The reader's invites as the fake holds them, newest first: all of them, in one page of the server's most. */
export const heldInvites = async (fake: FakeApi): Promise<OwnInvite[]> =>
  (await heldBy<{ items: OwnInvite[] }>(fake, "/invites?limit=40")).items;

/** The reader's invite at `index` of the fake's list. */
export async function heldInvite(fake: FakeApi, index: number): Promise<OwnInvite> {
  const invite = (await heldInvites(fake)).find((held) => held.id === fake.inviteId(index));
  if (invite === undefined) throw new Error(`test: the fake holds no invite at ${String(index)}`);
  return invite;
}

/**
 * The problem a server answers a new invite of `body` with for a reader who stands as `options` say (a 403 for one
 * who may not create invites, a 409 at the limit, a 422 for a value over the server's most): asked of a fake of its
 * own, for a test to fail a write with on another.
 */
export async function refusalOn(
  options: FakeOptions,
  body: { max_uses?: number; expires_in?: number } = {},
): Promise<Exclude<Failure, number | "network">> {
  const answer = await createFakeApi(options).serve(
    new Request(`${fakeOrigin}/api/v1/invites`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
  if (answer.ok) throw new Error("test: the fake created the invite it was to refuse");
  return (await answer.json()) as Exclude<Failure, number | "network">;
}

/** The headings of the screen in their order: its title, then its sections'. */
export const headings = (): string[] =>
  within(screen.getByRole("main"))
    .queryAllByRole("heading")
    .map((heading) => heading.textContent);

/** Invites, the entry, on a server whose account stands as `access` says, before its reads answered. */
export const invitesDrawn = (access: Access = {}, given: Given = {}) =>
  settingsOn(access, (address) => <SettingsEntryScreen entry="invites" languages={["en", "ja"]} {...address} />, given);

/** Invites with its list drawn: every read answered. */
export async function invitesRead(access: Access = {}, given: Given = {}) {
  const on = await invitesDrawn(access, given);
  await screen.findByRole("region", { name: /^Your invites(\s*\d+)?$/ });
  await atRest(on);
  return on;
}

/**
 * The screen of the reader's invite at `index` of the fake's list (one past its invites names none), before its
 * reads answered.
 */
export function inviteDrawn(index: number, access: Access = {}, given: Given = {}) {
  let id = "";
  return settingsOn(access, (address) => <OwnInviteScreen id={id} {...address} />, {
    ...given,
    before: (on) => {
      // The fake names its invites from the moment it started: the screen is given the ID once the fake stands.
      id = on.fake.inviteId(index);
      given.before?.(on);
    },
  });
}

/** The screen of the reader's invite at `index`, with the invite drawn: every read answered. */
export async function inviteRead(index: number, access: Access = {}, given: Given = {}) {
  const on = await inviteDrawn(index, access, given);
  await screen.findByRole("region", { name: "Status" });
  await atRest(on);
  return on;
}

/** The screen that creates an invite, before its reads answered. */
export const createDrawn = (access: Access = {}, given: Given = {}) =>
  settingsOn(access, (address) => <InviteCreate {...address} />, given);

/** The screen that creates an invite, with its form drawn: every read answered. */
export async function createForm(access: Access = {}, given: Given = {}) {
  const on = await createDrawn(access, given);
  await screen.findByRole("textbox", { name: "People who can sign up" });
  await atRest(on);
  return on;
}
