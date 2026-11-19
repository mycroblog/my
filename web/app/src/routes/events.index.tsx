// SPDX-License-Identifier: AGPL-3.0-only

// The upcoming events the reader sees, at /events, a place of the menu (ADR-0161 §9): the nearest first, each leading to
// its page. Only this server's people see events: without a session the screen asks the reader to sign in.

import { EventsScreen, loadEvents } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/events/")({
  loader: ({ context }) => screenLoad(context, loadEvents),
  component: EventsPage,
  // A place of the menu opens at its top, where the nearest events come first.
  staticData: { scroll: "top" },
});

function EventsPage() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.events_place")}</ScreenHeading>
      {data === null ? <SignedOut returnTo="/events" /> : <EventsScreen />}
    </>
  );
}
