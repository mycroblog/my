// SPDX-License-Identifier: AGPL-3.0-only

// What a screen a visitor reads too gives its feature: the app's sign-in, coming back to the address the feature names
// (a hashtag, a profile, a list of follows, a post), which the actions that need an account lead to (ADR-0090 §16).

import { useRouter } from "@tanstack/react-router";
import { signInReturningTo } from "../sign-in";

/** The sign-in that comes back to `returnTo`, for a reader without a session (`signedIn` false); none with one. */
export function useVisitorSignIn(signedIn: boolean): ((returnTo: string) => void) | undefined {
  const router = useRouter();
  if (signedIn) return undefined;
  return (returnTo) => {
    void router.navigate({ href: signInReturningTo(returnTo) });
  };
}
