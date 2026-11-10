// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { InteractionRequests, interactionRequestsQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** The replies, boosts, and quotes of the user's posts that await the user's approval (ADR-0155 §10). */
export const Route = createFileRoute("/settings/interaction-requests")({
  staticData: { title: "client:interaction_requests.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    context.queryClient.infiniteQuery({ ...interactionRequestsQueryOptions(context.api), staleTime: Infinity }),
  component: InteractionRequestsPage,
});

function InteractionRequestsPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:interaction_requests.title")}</SettingsHeading>
      <InteractionRequests />
    </>
  );
}
