// SPDX-License-Identifier: AGPL-3.0-only

// The form that changes an event, at /events/{id}/edit, which the event's menu leads its organizers to (ADR-0161 §4,
// §5): Save, and back, go to the event's page. Without a session it asks the reader to sign in.

import { EventEditScreen, loadEvent } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/events/$id_/edit")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadEvent(data, params.id)),
  component: EditScreen,
  // A form of its own opens at its top.
  staticData: { scroll: "top" },
});

function EditScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.event_edit")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <EventEditScreen
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
