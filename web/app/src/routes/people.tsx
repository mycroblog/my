// SPDX-License-Identifier: AGPL-3.0-only

// Finding people, at /people (FR-308): the accounts the server suggests the reader follow, each with its follow. A
// one-person server suggests no one, and says so. Without a session it asks the reader to sign in.

import { FindPeople, loadPeople } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/people")({
  loader: ({ context }) => screenLoad(context, loadPeople),
  component: PeopleScreen,
  // A place of the navigation opens at its top.
  staticData: { scroll: "top" },
});

function PeopleScreen() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.people")}</ScreenHeading>
      {data === null ? <SignedOut /> : <FindPeople />}
    </>
  );
}
