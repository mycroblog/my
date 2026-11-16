// SPDX-License-Identifier: AGPL-3.0-only

// A list's members, at /lists/{id}/members (ADR-0013 §11, §15), which the list's menu opens: back goes to the list's
// timeline. Without a session it asks the reader to sign in.

import { ListMembers, loadListMembers } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/lists/$id_/members")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadListMembers(data, params.id)),
  component: MembersScreen,
  // A screen of its own opens at its top, where the field that adds is.
  staticData: { scroll: "top" },
});

function MembersScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.list")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <ListMembers
          key={id}
          id={id}
          onBack={() => {
            void router.navigate({ to: "/lists/$id", params: { id } });
          }}
        />
      )}
    </>
  );
}
