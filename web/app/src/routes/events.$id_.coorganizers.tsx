// SPDX-License-Identifier: AGPL-3.0-only

// The co-organizers of an event, at /events/{id}/coorganizers, which the event's menu leads its organizer to
// (ADR-0161 §4, FR-944): back goes to the event's page. Without a session it asks the reader to sign in.

import { EventCoorganizersScreen, loadEvent } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/events/$id_/coorganizers")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadEvent(data, params.id)),
  component: CoorganizersScreen,
  staticData: { scroll: "top" },
});

function CoorganizersScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.event_coorganizers")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <EventCoorganizersScreen
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
