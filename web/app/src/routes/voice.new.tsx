// SPDX-License-Identifier: AGPL-3.0-only

// The form that opens a voice chat room, at /voice/new, which the rooms' screen leads to for a reader who may open
// rooms (ADR-0100 §2, §3, §9): opening it joins the reader to the room they host, whose page takes the form's place in
// the history. Without a session it asks the reader to sign in.

import { voiceOpenPath } from "@my/app-components";
import { VoiceRoomOpen } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";
import { useNarrowViewport } from "../shell/viewport";

export const Route = createFileRoute("/voice/new")({
  component: OpenScreen,
  // A form of its own opens at its top.
  staticData: { scroll: "top" },
});

function OpenScreen() {
  const { data } = Route.useRouteContext();
  const router = useRouter();
  const sheet = useNarrowViewport();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.voice_open")}</ScreenHeading>
      {data === null ? (
        <SignedOut returnTo={voiceOpenPath} />
      ) : (
        <VoiceRoomOpen
          data={data}
          sheet={sheet}
          onOpened={(id) => {
            void router.navigate({ to: "/voice/$id", params: { id }, replace: true });
          }}
          onBack={() => {
            void router.navigate({ to: "/voice" });
          }}
        />
      )}
    </>
  );
}
