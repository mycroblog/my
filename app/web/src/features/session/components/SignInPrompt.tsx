// SPDX-License-Identifier: AGPL-3.0-only
import { type HeadingLevel, SignInState } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { signInPath } from "../../../lib/paths";

export type SignInPromptProps = {
  /** The short fact ("You are not signed in"). */
  heading: string;
  /** What signing in shows here. */
  description: string;
  /** The page the sign-in comes back to. */
  returnTo: string;
  headingLevel?: HeadingLevel;
};

/**
 * The region of a page that needs a session, while nobody is signed in: `SignInState` with the way to sign in and come
 * back to `returnTo` (web-client/10-api-client-and-auth.md §10). Every such region uses it, so they all look alike (P4).
 */
export function SignInPrompt({ heading, description, returnTo, headingLevel }: SignInPromptProps) {
  const t = useT();
  return (
    <SignInState
      heading={heading}
      description={description}
      {...(headingLevel !== undefined && { headingLevel })}
      signInHref={signInPath(returnTo)}
      signInLabel={t("client:shell.sign_in_required.sign_in")}
    />
  );
}
