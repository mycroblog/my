// SPDX-License-Identifier: AGPL-3.0-only

// The screen of an account's lists of follows, at /@user/following and /@user/followers: headed by the account's name
// and handle, a tab each, choosing a tab opening its address in place of the other's. Back goes to where the reader
// came from, or to the profile. A visitor reads the lists the account shows everyone, and a row's Follow leads them to
// the app's sign-in, which comes back to the list (ADR-0090 §16).

import { Follows } from "@my/app-features";
import { useCanGoBack, useRouter } from "@tanstack/react-router";
import { ProfileHeading } from "./profile-heading";
import { useVisitorSignIn } from "./visitor";

/** The lists of the account of `acct`, the tab of `list` chosen; read by a visitor too (`signedIn` false). */
export function FollowsScreen({
  signedIn,
  acct,
  list,
}: {
  signedIn: boolean;
  acct: string;
  list: "following" | "followers";
}) {
  const router = useRouter();
  const canGoBack = useCanGoBack();
  const signIn = useVisitorSignIn(signedIn);
  return (
    <>
      <ProfileHeading acct={acct} signedIn={signedIn} />
      <Follows
        key={acct}
        acct={acct}
        list={list}
        signIn={signIn}
        onList={(to) => {
          void router.navigate({ to: `/@{$acct}/${to}`, params: { acct }, replace: true });
        }}
        onBack={() => {
          if (canGoBack) router.history.back();
          else void router.navigate({ to: "/@{$acct}", params: { acct } });
        }}
      />
    </>
  );
}
