// SPDX-License-Identifier: AGPL-3.0-only

// A profile, at /@user (a local account) or /@user@domain (a remote one): the server's address of an account
// (ADR-0018), which its HTML shell serves with the profile's link preview. A visitor reads it too, as the server shows
// a profile and its public posts to everyone; the actions that need an account lead them to the app's sign-in, which
// comes back to the profile or the post (ADR-0090 §16). The profile of an account that is not there goes back to
// where the reader came from, or home.

import { loadProfile, Profile } from "@my/app-features";
import { createFileRoute, useCanGoBack, useRouter } from "@tanstack/react-router";
import { screenLoad } from "../../screen-load";
import { ProfileHeading } from "../../shell/profile-heading";
import { useVisitorSignIn } from "../../shell/visitor";

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
  const signIn = useVisitorSignIn(data !== null);
  return (
    <>
      <ProfileHeading acct={acct} signedIn={data !== null} />
      <Profile
        key={acct}
        acct={acct}
        signIn={signIn}
        onBack={() => {
          if (canGoBack) router.history.back();
          else void router.navigate({ to: "/" });
        }}
      />
    </>
  );
}
