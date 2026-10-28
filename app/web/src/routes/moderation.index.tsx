// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { ModerationOverview } from "../features/moderation";
import { viewerQueryOptions } from "../features/session";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The moderation overview that the navigation's Moderation entry opens (ADR-0058, screens/moderation.md): what
 * waits for the moderators (FR-712) and the reports to handle first. The loader reads the viewer's account, whose
 * permissions decide what it shows.
 */
export const Route = createFileRoute("/moderation/")({
  staticData: { title: "client:admin.moderation.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api } }) => {
    await queryClient.query({ ...viewerQueryOptions(api), staleTime: Infinity });
  },
  component: ModerationPage,
});

function ModerationPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.moderation.title")}</PageHeading>
      <ModerationOverview />
    </>
  );
}
