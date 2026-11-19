// SPDX-License-Identifier: AGPL-3.0-only

// The invitations of an open voice chat room, at /voice/{id}/invitations, which the room's menu leads its host and
// co-hosts to (ADR-0100 §7): back goes to the room's page. Without a session it asks the reader to sign in.

import { loadVoiceRoom, VoiceInvitations } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/voice/$id_/invitations")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadVoiceRoom(data, params.id)),
  component: InvitationsScreen,
  // A screen of its own opens at its top, where the search is.
  staticData: { scroll: "top" },
});

function InvitationsScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.voice_invitations")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <VoiceInvitations
          key={id}
          data={data}
          id={id}
          onBack={() => {
            void router.navigate({ to: "/voice/$id", params: { id } });
          }}
        />
      )}
    </>
  );
}
