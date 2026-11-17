// SPDX-License-Identifier: AGPL-3.0-only

// What a screen that needs a session shows without one: that the reader signs in to see it, with the app's sign-in
// (/login), which leaves the app for the server's sign-in (ADR-0090 §16) and comes back to the app's start, or to
// `returnTo`, the address of a screen a link from outside the app opened (a push's notification).

import { FeedNotice, TextLink } from "@my/design-system";
import { useStrings } from "../locale";
import { signInPath, signInReturningTo } from "../sign-in";

export function SignedOut({ returnTo }: { returnTo?: string | undefined }) {
  const t = useStrings();
  const href = returnTo === undefined ? signInPath : signInReturningTo(returnTo);
  return (
    <FeedNotice action={<TextLink href={href}>{t("app.sign_in")}</TextLink>}>{t("app.signed_out_screen")}</FeedNotice>
  );
}
