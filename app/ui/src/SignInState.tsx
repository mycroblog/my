// SPDX-License-Identifier: AGPL-3.0-only
// What a region shows instead of its content while nobody is signed in and the content needs a session (the home
// timeline, the notifications, a timeline the server keeps for signed-in users): the one empty state of that kind
// (web-client/16-look-states-and-wording.md G14), so that every page says it the same way (P4) and leads on to sign in.
import type { ReactNode } from "react";
import { EmptyState, type HeadingLevel } from "./EmptyState";
import { SignInIcon } from "./Icons";
import { Link } from "./Link";

export type SignInStateProps = {
  /** The short fact ("You are not signed in"). */
  heading: ReactNode;
  /** The heading's level: one below the heading of the region it fills. */
  headingLevel?: HeadingLevel;
  /** What signing in shows here ("Sign in to read the posts of the people you follow."). */
  description: ReactNode;
  /** The sign-in page, which comes back to this page afterwards. */
  signInHref: string;
  /** The link's text ("Sign in"). */
  signInLabel: string;
};

/** The signed-out state of a region: the sign-in icon, a heading, what signing in shows, and the link to sign in. */
export function SignInState({ heading, headingLevel, description, signInHref, signInLabel }: SignInStateProps) {
  return (
    <EmptyState
      heading={heading}
      {...(headingLevel !== undefined && { headingLevel })}
      description={description}
      illustration={<SignInIcon />}
      action={
        <Link href={signInHref} variant="standalone">
          {signInLabel}
        </Link>
      }
    />
  );
}
