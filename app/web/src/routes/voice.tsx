// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { VoiceRoomList, voiceRoomsQueryOptions } from "../features/voice";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The open voice chat rooms that the user sees, and starting one (ADR-0100 §8). Voice chat is for the users of this
 * server, so it needs a session.
 */
export const Route = createFileRoute("/voice")({
  staticData: { title: "client:voice.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api } }) => {
    // A failure does not fail the page: the page shows it, or voice chat off (404), in its body.
    await queryClient.infiniteQuery({ ...voiceRoomsQueryOptions(api), staleTime: Infinity }).catch(() => undefined);
  },
  component: VoicePage,
});

function VoicePage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:voice.title")}</PageHeading>
      <VoiceRoomList />
    </>
  );
}
