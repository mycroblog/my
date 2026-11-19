// SPDX-License-Identifier: AGPL-3.0-only

// The open voice chat rooms, at /voice, a place of the menu while the server has voice chat (ADR-0100 §8): the rooms
// the reader sees, in the server's order, each leading to its page. Without a session it asks the reader to sign in.

import { loadVoiceRooms, VoiceRooms } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/voice/")({
  loader: ({ context }) => screenLoad(context, loadVoiceRooms),
  component: VoiceRoomsScreen,
  // A place of the menu opens at its top, where the rooms come first.
  staticData: { scroll: "top" },
});

function VoiceRoomsScreen() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.voice_rooms")}</ScreenHeading>
      {data === null ? <SignedOut returnTo="/voice" /> : <VoiceRooms />}
    </>
  );
}
