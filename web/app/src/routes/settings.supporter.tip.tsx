// SPDX-License-Identifier: AGPL-3.0-only

// Tipping the server, at /settings/supporter/tip, a screen under the settings' Supporter (ADR-0162, FR-918), where the
// tip's checkout returns (`?tip=<id>&checkout=…`). Back, in its head, goes to Supporter. Without a session it asks the
// reader to sign in.

import { loadSupporterTip, SupporterTip, useSupporterTipTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/supporter/tip")({
  loader: ({ context }) => screenLoad(context, loadSupporterTip),
  component: TipScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function TipScreen() {
  const { data } = Route.useRouteContext();
  return (
    <SettingsUnder entry="supporter" title={useSupporterTipTitle()} signedIn={data !== null} Screen={SupporterTip} />
  );
}
