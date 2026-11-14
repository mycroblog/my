// SPDX-License-Identifier: AGPL-3.0-only

// The home screen, at /: the home timeline of the signed-in reader. Without a session it asks the reader to sign in
// on the server's sign-in page, which a full page load opens (ADR-0090 §16); the app's own sign-in is #2899's.

import { HomeTimeline, loadHomeTimeline } from "@my/app-features";
import { FeedNotice } from "@my/design-system";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";

/** The server's sign-in page (internal/interface/authpages). */
const signInPath = "/auth/sign_in";

export const Route = createFileRoute("/")({
  loader: ({ context }) => (context.data === null ? undefined : loadHomeTimeline(context.data)),
  component: Home,
  // Back on the home timeline, the reader is where they left it.
  staticData: { scroll: "restore" },
});

function Home() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.home")}</ScreenHeading>
      {data === null ? (
        <FeedNotice action={<a href={signInPath}>{t("app.sign_in")}</a>}>{t("app.signed_out")}</FeedNotice>
      ) : (
        <HomeTimeline />
      )}
    </>
  );
}
