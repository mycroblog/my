// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { AuthorizedApps, authorizedAppsQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** Authorized apps (ADR-0090 §5): the apps with access to the account, and revoking them. */
export const Route = createFileRoute("/settings/authorized-apps")({
  staticData: { title: "client:settings.authorized_apps.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context }) => {
    await context.queryClient.query({ ...authorizedAppsQueryOptions(context.api), staleTime: Infinity });
  },
  component: AuthorizedAppsPage,
});

function AuthorizedAppsPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.authorized_apps.title")}</SettingsHeading>
      <AuthorizedApps />
    </>
  );
}
