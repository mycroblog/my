// SPDX-License-Identifier: AGPL-3.0-only

// What a screen that needs a session shows without one: that the reader signs in to see it, with the app's sign-in
// (/login), which a full page load opens (ADR-0090 §16).

import { FeedNotice } from "@my/design-system";
import { useStrings } from "../locale";
import { signInPath } from "../sign-in";

export function SignedOut() {
  const t = useStrings();
  return <FeedNotice action={<a href={signInPath}>{t("app.sign_in")}</a>}>{t("app.signed_out_screen")}</FeedNotice>;
}
