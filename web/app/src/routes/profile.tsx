// SPDX-License-Identifier: AGPL-3.0-only

// The reader's own profile, at /profile, a place of the navigation; the same profile as at its own address, /@user.
// Without a session it asks the reader to sign in.

import { loadOwnProfile, OwnProfile } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ProfileHeading } from "../shell/profile-heading";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/profile")({
  loader: ({ context }) => (context.data === null ? undefined : loadOwnProfile(context.data)),
  component: OwnProfileScreen,
  // Back on the profile, the reader is where they left it.
  staticData: { scroll: "restore" },
});

function OwnProfileScreen() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  if (data === null)
    return (
      <>
        <ScreenHeading>{t("app.profile")}</ScreenHeading>
        <SignedOut />
      </>
    );
  return (
    <>
      <ProfileHeading acct={undefined} />
      <OwnProfile />
    </>
  );
}
