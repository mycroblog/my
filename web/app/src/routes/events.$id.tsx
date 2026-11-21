// SPDX-License-Identifier: AGPL-3.0-only

// An event's page, at /events/{id} (ADR-0161 §9), where the list, the column beside the timeline, the organizer's
// profile and an event's notifications lead; `?tab=board` opens it on its Board tab (§17), where a notification of a
// new post on the board leads. Choosing a tab puts its address in place of the other's, as a change of the search alone
// (no move of the screen). A visitor, signed out, is told that only this server's people see events, with the sign-in,
// which comes back here. Joining its open voice room is the call's (joinRoom), from the reader's press, and then the
// room's page shows.

import { eventBoardPath, eventPath, voiceRoomPath } from "@my/app-components";
import { EventScreen, EventVisitor, joinRoom, loadEvent, useEventTitle } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { signInReturningTo } from "../sign-in";

export const Route = createFileRoute("/events/$id")({
  // Any other tab is Details: the address's own value is set aside, since the router lays this over the raw search.
  validateSearch: (search: Record<string, unknown>): { tab?: "board" | undefined } => ({
    tab: search["tab"] === "board" ? "board" : undefined,
  }),
  loaderDeps: ({ search }) => ({ tab: search.tab }),
  loader: ({ context, params, deps }) => screenLoad(context, (data) => loadEvent(data, params.id, deps.tab)),
  component: EventPage,
  // A page of its own opens at its top.
  staticData: { scroll: "top" },
});

function Heading({ id, signedIn }: { id: string; signedIn: boolean }) {
  const t = useStrings();
  return <ScreenHeading>{useEventTitle(id, signedIn) ?? t("app.event")}</ScreenHeading>;
}

function EventPage() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const { tab } = Route.useSearch();
  const router = useRouter();
  return (
    <>
      <Heading id={id} signedIn={data !== null} />
      {data === null ? (
        <EventVisitor
          onSignIn={() => {
            void router.navigate({ href: signInReturningTo(tab === "board" ? eventBoardPath(id) : eventPath(id)) });
          }}
        />
      ) : (
        <EventScreen
          key={id}
          id={id}
          tab={tab === "board" ? "board" : "details"}
          onTab={(to) => {
            void router.navigate({ href: to === "board" ? eventBoardPath(id) : eventPath(id), replace: true });
          }}
          join={async (roomId) => {
            await joinRoom(data, roomId);
            await router.navigate({ href: voiceRoomPath(roomId) });
          }}
          onBack={() => {
            void router.navigate({ to: "/events" });
          }}
        />
      )}
    </>
  );
}
