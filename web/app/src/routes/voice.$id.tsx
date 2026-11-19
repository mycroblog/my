// SPDX-License-Identifier: AGPL-3.0-only

// A voice chat room's page, at /voice/{id} (ADR-0100 §9), where the list, the column beside the timeline, a hashtag's
// page, a profile, a notification and the call's bar lead: the room before the reader joins, while they are in it, and
// once it ended. A visitor, signed out or of another server, gets what the room's summary says and the sign-in, which
// comes back here. The call itself is the feature's and outlives the page (calls.ts).

import { loadVoiceRoom, useVoiceRoomTitle, VoiceRoomScreen, VoiceRoomVisitor } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { useNarrowViewport } from "../shell/viewport";
import { signInReturningTo } from "../sign-in";

export const Route = createFileRoute("/voice/$id")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadVoiceRoom(data, params.id)),
  component: VoiceRoomPage,
  // A page of its own opens at its top.
  staticData: { scroll: "top" },
});

function Heading({ id, signedIn }: { id: string; signedIn: boolean }) {
  const t = useStrings();
  return <ScreenHeading>{useVoiceRoomTitle(id, signedIn) ?? t("app.voice_room")}</ScreenHeading>;
}

function VoiceRoomPage() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const sheet = useNarrowViewport();
  return (
    <>
      <Heading id={id} signedIn={data !== null} />
      {data === null ? (
        <VoiceRoomVisitor
          key={id}
          id={id}
          onSignIn={() => {
            void router.navigate({ href: signInReturningTo(`/voice/${id}`) });
          }}
        />
      ) : (
        <VoiceRoomScreen
          key={id}
          data={data}
          id={id}
          sheet={sheet}
          onBack={() => {
            void router.navigate({ to: "/voice" });
          }}
        />
      )}
    </>
  );
}
