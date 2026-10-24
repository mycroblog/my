// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { OAuthAppSettings, oauthAppsQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** Development (ADR-0090): the user's own OAuth apps, their secrets, and tokens issued to the user. */
export const Route = createFileRoute("/settings/apps")({
  staticData: { title: "client:settings.apps.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context }) => {
    await context.queryClient.query({ ...oauthAppsQueryOptions(context.api), staleTime: Infinity });
  },
  component: OAuthAppsPage,
});

function OAuthAppsPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.apps.title")}</SettingsHeading>
      <OAuthAppSettings />
    </>
  );
}
