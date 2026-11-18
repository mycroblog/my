// SPDX-License-Identifier: AGPL-3.0-only

// An invite of the reader's, at /settings/invites/{id}, a screen under the settings' Invites (ADR-0106 §18): its link
// to copy while it is usable, what it allows now, and revoking it. Back, in its head, goes to Invites. Without a
// session it asks the reader to sign in.

import { loadInvitesSettings, OwnInviteScreen, useOwnInviteTitle, type SettingsSubProps } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/invites/$id")({
  loader: ({ context }) => screenLoad(context, loadInvitesSettings),
  component: InviteScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The screen of the invite the address names. */
function Screen(props: SettingsSubProps) {
  const { id } = Route.useParams();
  return <OwnInviteScreen key={id} id={id} {...props} />;
}

/** The screen under the invite's code, for a signed-in reader, whose invites alone are read. */
function Titled() {
  const { id } = Route.useParams();
  return <SettingsUnder entry="invites" title={useOwnInviteTitle(id)} signedIn Screen={Screen} />;
}

function InviteScreen() {
  const { data } = Route.useRouteContext();
  // Without a session nothing of the reader's is read: the title is the settings' own.
  return data === null ? <SettingsUnder entry="invites" title="" signedIn={false} Screen={Screen} /> : <Titled />;
}
