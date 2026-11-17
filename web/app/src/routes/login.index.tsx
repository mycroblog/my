// SPDX-License-Identifier: AGPL-3.0-only

// The app's sign-in, at /login (ADR-0090 §16): it starts the authorization code flow with PKCE and leaves the app for
// the server's auth screens by a page load, coming back to /login/callback and then to `return_to`. A browser that is
// signed in already goes back at once.

import { appPath, startSignIn } from "@my/app-data";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";
import { signInOptions } from "../sign-in";

export const Route = createFileRoute("/login/")({
  // A page load leaves it at once; there is no position to bring back.
  staticData: { scroll: "top" },
  validateSearch: (search: Record<string, unknown>): { return_to?: string } =>
    typeof search["return_to"] === "string" ? { return_to: search["return_to"] } : {},
  loaderDeps: ({ search }) => ({ returnTo: search.return_to }),
  loader: async ({ context, deps }) => {
    if (context.data !== null) {
      location.replace(appPath(deps.returnTo));
      return;
    }
    location.assign(await startSignIn(signInOptions(context.config), deps.returnTo));
  },
  component: SigningIn,
});

function SigningIn() {
  const t = useStrings();
  return <ScreenHeading>{t("app.sign_in")}</ScreenHeading>;
}
