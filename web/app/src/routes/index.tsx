// SPDX-License-Identifier: AGPL-3.0-only

// The home screen, at /: the home timeline of the signed-in reader. Without a session it asks the reader to sign in
// with the app's sign-in (/login), which a full page load opens, so that it leaves the app for the server's auth
// screens and comes back with the session (ADR-0090 §16).

import { HomeTimeline, loadHomeTimeline } from "@my/app-features";
import { FeedNotice } from "@my/design-system";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { useCompose } from "../shell/compose";
import { ScreenHeading } from "../shell/heading";
import { signInPath } from "../sign-in";

export const Route = createFileRoute("/")({
  loader: ({ context }) => (context.data === null ? undefined : loadHomeTimeline(context.data)),
  component: Home,
  // Back on the home timeline, the reader is where they left it.
  staticData: { scroll: "restore" },
});

function Home() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  const { open } = useCompose();
  return (
    <>
      <ScreenHeading>{t("app.home")}</ScreenHeading>
      {data === null ? (
        <FeedNotice action={<a href={signInPath}>{t("app.sign_in")}</a>}>{t("app.signed_out")}</FeedNotice>
      ) : (
        <HomeTimeline onCompose={open} />
      )}
    </>
  );
}
