// SPDX-License-Identifier: AGPL-3.0-only

// The settings of an open voice chat room, at /voice/{id}/settings, which the room's menu leads its host to (ADR-0100
// §3): Save, and back, go to the room's page. Without a session it asks the reader to sign in.

import { loadVoiceRoom, VoiceRoomSettingsScreen } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";
import { useNarrowViewport } from "../shell/viewport";

export const Route = createFileRoute("/voice/$id_/settings")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadVoiceRoom(data, params.id)),
  component: SettingsScreen,
  // A form of its own opens at its top.
  staticData: { scroll: "top" },
});

function SettingsScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const sheet = useNarrowViewport();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.voice_room_settings")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <VoiceRoomSettingsScreen
          key={id}
          data={data}
          id={id}
          sheet={sheet}
          onBack={() => {
            void router.navigate({ to: "/voice/$id", params: { id } });
          }}
        />
      )}
    </>
  );
}
