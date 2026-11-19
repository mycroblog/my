// SPDX-License-Identifier: AGPL-3.0-only

// The root of the server's administration, at /admin, a place of the navigation for a reader with a permission of
// the server's administration (ADR-0058 §2): the screens their permissions open, or why there is none. Without a
// session it asks the reader to sign in.

import { useRights } from "@my/app-data";
import { StaffRoot } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/admin/")({
  component: AdminRoot,
  // A place of the navigation opens at its top.
  staticData: { scroll: "top" },
});

function Root() {
  return <StaffRoot area="admin" rights={useRights().admin} />;
}

function AdminRoot() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.admin")}</ScreenHeading>
      {data === null ? <SignedOut returnTo="/admin" /> : <Root />}
    </>
  );
}
