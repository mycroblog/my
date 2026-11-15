// SPDX-License-Identifier: AGPL-3.0-only

// What a screen that needs a session shows without one: that the reader signs in to see it, with the app's sign-in
// (/login), which leaves the app for the server's sign-in (ADR-0090 §16).

import { FeedNotice, TextLink } from "@my/design-system";
import { useStrings } from "../locale";
import { signInPath } from "../sign-in";

export function SignedOut() {
  const t = useStrings();
  return (
    <FeedNotice action={<TextLink href={signInPath}>{t("app.sign_in")}</TextLink>}>
      {t("app.signed_out_screen")}
    </FeedNotice>
  );
}
