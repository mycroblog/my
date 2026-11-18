// SPDX-License-Identifier: AGPL-3.0-only

// One list of the settings' entry Relationships as a whole, at /settings/relationships/{list} (blocked, muted, servers,
// hashtags), the screen under the entry (ADR-0110 §5) that its section's door leads to: the list a page at a time;
// back goes to the entry. An address that names no list says so in the screen, under the settings' name. Without a
// session it asks the reader to sign in.

import { loadRelationshipSettings, RelationshipListScreen, useRelationshipListTitle } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/settings/relationships/$list")({
  loader: ({ context }) => screenLoad(context, loadRelationshipSettings),
  component: RelationshipListRoute,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function RelationshipListRoute() {
  const { data } = Route.useRouteContext();
  const { list } = Route.useParams();
  const router = useRouter();
  const t = useStrings();
  const title = useRelationshipListTitle(list);
  return (
    <>
      <ScreenHeading>{title ?? t("app.settings")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <RelationshipListScreen
          key={list}
          list={list}
          onBack={() => {
            void router.navigate({ to: "/settings/$entry", params: { entry: "relationships" } });
          }}
        />
      )}
    </>
  );
}
