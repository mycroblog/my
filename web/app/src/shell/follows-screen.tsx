// SPDX-License-Identifier: AGPL-3.0-only

// The screen of an account's lists of follows, at /@user/following and /@user/followers: headed by the account's name
// and handle, a tab each, choosing a tab opening its address in place of the other's. Back goes to where the reader
// came from, or to the profile. Without a session it asks the reader to sign in.

import { Follows } from "@my/app-features";
import { useCanGoBack, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ScreenHeading } from "./heading";
import { ProfileHeading } from "./profile-heading";
import { SignedOut } from "./signed-out";

/** The lists of the account of `acct`, the tab of `list` chosen; a sign-in for a reader without a session. */
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
  const t = useStrings();
  if (!signedIn)
    return (
      <>
        <ScreenHeading>{t("app.profile")}</ScreenHeading>
        <SignedOut />
      </>
    );
  return (
    <>
      <ProfileHeading acct={acct} />
      <Follows
        key={acct}
        acct={acct}
        list={list}
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
