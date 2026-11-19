// SPDX-License-Identifier: AGPL-3.0-only

// The invitations of an event, at /events/{id}/invitations, which the event's menu leads its organizers to (ADR-0161
// §4, FR-944): back goes to the event's page. Without a session it asks the reader to sign in.

import { EventInvitationsScreen, loadEvent } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/events/$id_/invitations")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadEvent(data, params.id)),
  component: InvitationsScreen,
  staticData: { scroll: "top" },
});

function InvitationsScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.event_invitations")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <EventInvitationsScreen
          key={id}
          id={id}
          onBack={() => {
            void router.navigate({ to: "/events/$id", params: { id } });
          }}
        />
      )}
    </>
  );
}
