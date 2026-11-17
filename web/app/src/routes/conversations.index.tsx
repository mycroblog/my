// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages, at /conversations, a place of the navigation (FR-212, ADR-0110 §11): the reader's conversations,
// newest first. Without a session it asks the reader to sign in.

import { ConversationList, loadConversations } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/conversations/")({
  loader: ({ context }) => screenLoad(context, loadConversations),
  component: ConversationsScreen,
  // A place of the navigation opens at its top, where the newest conversations are.
  staticData: { scroll: "top" },
});

function ConversationsScreen() {
  const { data } = Route.useRouteContext();
  const router = useRouter();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.direct_messages")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <ConversationList
          onOpen={(href) => {
            void router.navigate({ href });
          }}
        />
      )}
    </>
  );
}
