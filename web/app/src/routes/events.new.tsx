// SPDX-License-Identifier: AGPL-3.0-only

// The form that creates an event, at /events/new, which the events' screen leads to for a reader who may create
// events (ADR-0161 §3, FR-940): the event's page takes the form's place in the history once it is created. Without a
// session it asks the reader to sign in.

import { eventNewPath } from "@my/app-components";
import { loadMayCreateEvents } from "@my/app-data";
import { EventCreateScreen } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/events/new")({
  // Whether the reader may create events, read before the form, so that it opens knowing.
  loader: ({ context }) => screenLoad(context, loadMayCreateEvents),
  component: CreateScreen,
  // A form of its own opens at its top.
  staticData: { scroll: "top" },
});

function CreateScreen() {
  const { data } = Route.useRouteContext();
  const router = useRouter();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.event_new")}</ScreenHeading>
      {data === null ? (
        <SignedOut returnTo={eventNewPath} />
      ) : (
        <EventCreateScreen
          onCreated={(id) => {
            void router.navigate({ to: "/events/$id", params: { id }, replace: true });
          }}
          onBack={() => {
            void router.navigate({ to: "/events" });
          }}
        />
      )}
    </>
  );
}
