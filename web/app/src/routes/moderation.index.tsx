// SPDX-License-Identifier: AGPL-3.0-only

// The root of the moderators' screens, at /moderation, a place of the navigation for a reader with a moderation
// permission (ADR-0058 §2): the screens their permissions open, or why there is none. Without a session it asks the
// reader to sign in.

import { useRights } from "@my/app-data";
import { StaffRoot } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/moderation/")({
  component: ModerationRoot,
  // A place of the navigation opens at its top.
  staticData: { scroll: "top" },
});

function Root() {
  return <StaffRoot area="moderation" rights={useRights().moderation} />;
}

function ModerationRoot() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.moderation")}</ScreenHeading>
      {data === null ? <SignedOut returnTo="/moderation" /> : <Root />}
    </>
  );
}
