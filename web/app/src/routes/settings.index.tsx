// SPDX-License-Identifier: AGPL-3.0-only

// The settings' menu, at /settings, a place of the navigation (ADR-0110 §5, §11): the entries in their groups, each
// leading to its screen, and Sign out… at its end. Without a session it asks the reader to sign in.

import { SettingsIndex } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";
import { signOutHref } from "../shell/settings";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/settings/")({
  component: SettingsMenuScreen,
  // A place of the navigation opens at its top.
  staticData: { scroll: "top" },
});

function SettingsMenuScreen() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.settings")}</ScreenHeading>
      {data === null ? <SignedOut /> : <SettingsIndex signOutHref={signOutHref("/settings")} />}
    </>
  );
}
