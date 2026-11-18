// SPDX-License-Identifier: AGPL-3.0-only

// Move account, at /settings/account/move, the screen under the settings' entry Account and data (ADR-0110 §5) where
// the reader moves this account's followers to another account, and cancels the move once it has moved (FR-009); the
// closure's banner leads here. Back goes to the entry. Without a session it asks the reader to sign in.

import { AccountMoveScreen, loadAccountMoveSettings, useAccountMoveTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/account/move")({
  loader: ({ context }) => screenLoad(context, loadAccountMoveSettings),
  component: AccountMoveRoute,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function AccountMoveRoute() {
  const { data } = Route.useRouteContext();
  return (
    <SettingsUnder entry="account" title={useAccountMoveTitle()} signedIn={data !== null} Screen={AccountMoveScreen} />
  );
}
