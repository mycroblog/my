// SPDX-License-Identifier: AGPL-3.0-only

// A conversation of the direct messages, at /conversations/{id} (FR-212): its posts, oldest first, and the reply in
// place at its end, which the composer's feature draws; back, and hiding it, go to the list. Without a session it asks
// the reader to sign in.

import { ConversationView, InPlaceReply, loadConversation } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";
import { useNarrowViewport } from "../shell/viewport";

export const Route = createFileRoute("/conversations/$id")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadConversation(data, params.id)),
  component: ConversationScreen,
  // Back on a conversation, the reader is where they left it.
  staticData: { scroll: "restore" },
});

function ConversationScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const narrow = useNarrowViewport();
  const t = useStrings();
  const toList = () => {
    void router.navigate({ to: "/conversations" });
  };
  return (
    <>
      <ScreenHeading>{t("app.conversation")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <ConversationView
          key={id}
          id={id}
          onBack={toList}
          onHidden={toList}
          reply={(to) => <InPlaceReply {...to} pickerSheet={narrow} />}
        />
      )}
    </>
  );
}
