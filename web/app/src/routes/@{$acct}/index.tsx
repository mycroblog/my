// SPDX-License-Identifier: AGPL-3.0-only

// A profile, at /@user (a local account) or /@user@domain (a remote one): the server's address of an account
// (ADR-0018), which its HTML shell serves with the profile's link preview. Without a session it asks the reader to
// sign in. The profile of an account that is not there goes back to where the reader came from, or home.

import { loadProfile, Profile } from "@my/app-features";
import { createFileRoute, useCanGoBack, useRouter } from "@tanstack/react-router";
import { useStrings } from "../../locale";
import { screenLoad } from "../../screen-load";
import { ScreenHeading } from "../../shell/heading";
import { ProfileHeading } from "../../shell/profile-heading";
import { SignedOut } from "../../shell/signed-out";

export const Route = createFileRoute("/@{$acct}/")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadProfile(data, params.acct)),
  component: ProfileScreen,
  // Back on a profile, the reader is where they left it.
  staticData: { scroll: "restore" },
});

function ProfileScreen() {
  const { data } = Route.useRouteContext();
  const { acct } = Route.useParams();
  const router = useRouter();
  const canGoBack = useCanGoBack();
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
      <ProfileHeading acct={acct} />
      <Profile
        key={acct}
        acct={acct}
        onBack={() => {
          if (canGoBack) router.history.back();
          else void router.navigate({ to: "/" });
        }}
      />
    </>
  );
}
