// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { VoiceHistorySkeleton, VoiceHistoryView, voiceHistoryQueryOptions } from "../features/voice";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** The voice chat rooms the user hosted (ADR-0100 §13). */
export const Route = createFileRoute("/settings/voice")({
  staticData: { title: "client:voice.history.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api } }) => {
    // A failure does not fail the page: the page shows it, or voice chat off (404), in its body.
    await queryClient.infiniteQuery({ ...voiceHistoryQueryOptions(api), staleTime: Infinity }).catch(() => undefined);
  },
  component: VoiceHistoryPage,
  pendingComponent: VoiceHistoryPending,
});

function VoiceHistoryPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:voice.history.title")}</SettingsHeading>
      <VoiceHistoryView />
    </>
  );
}

/** The page's heading over the shape of the history, while its first page loads (web-client/16-look-states-and-wording.md G15). */
function VoiceHistoryPending() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:voice.history.title")}</SettingsHeading>
      <VoiceHistorySkeleton />
    </>
  );
}
