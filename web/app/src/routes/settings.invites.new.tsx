// SPDX-License-Identifier: AGPL-3.0-only

// Creating an invite, at /settings/invites/new, a screen under the settings' Invites (ADR-0106 §15, §18): the form,
// and once the invite is created its link, shown to copy. Back, in its head, goes to Invites. Without a session it
// asks the reader to sign in.

import { InviteCreate, loadInvitesSettings, useInviteCreateTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/invites/new")({
  loader: ({ context }) => screenLoad(context, loadInvitesSettings),
  component: CreateScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function CreateScreen() {
  const { data } = Route.useRouteContext();
  return (
    <SettingsUnder entry="invites" title={useInviteCreateTitle()} signedIn={data !== null} Screen={InviteCreate} />
  );
}
