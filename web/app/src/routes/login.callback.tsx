// SPDX-License-Identifier: AGPL-3.0-only

// The callback of the app's sign-in, at /login/callback (ADR-0090 §16): it trades the authorization response for the
// session and goes back to where the sign-in started with a page load, which opens the session (main.tsx). A response
// it cannot take says so, with a link that signs in again.

import { finishSignIn } from "@my/app-data";
import { FeedNotice } from "@my/design-system";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";
import { signInOptions, signInPath } from "../sign-in";

export const Route = createFileRoute("/login/callback")({
  // A page load leaves it at once; there is no position to bring back.
  staticData: { scroll: "top" },
  loader: async ({ context }) => {
    const returnTo = await finishSignIn(signInOptions(context.config), new URL(location.href));
    location.replace(returnTo);
  },
  component: SigningIn,
  errorComponent: Failed,
});

function SigningIn() {
  const t = useStrings();
  return <ScreenHeading>{t("app.sign_in")}</ScreenHeading>;
}

function Failed() {
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.sign_in")}</ScreenHeading>
      <FeedNotice action={<a href={signInPath}>{t("app.sign_in")}</a>}>{t("app.sign_in_failed")}</FeedNotice>
    </>
  );
}
