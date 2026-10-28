// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { PreferencesForm, preferencesQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/**
 * The display language, time zone, color theme (kept on this browser), default visibility, and timeline languages
 * (FR-007, FR-208). The notifications have their own page (/settings/notifications).
 */
export const Route = createFileRoute("/settings/preferences")({
  staticData: { title: "client:settings.preferences.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => context.queryClient.query({ ...preferencesQueryOptions(context.api), staleTime: Infinity }),
  component: PreferencesPage,
});

function PreferencesPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.preferences.title")}</SettingsHeading>
      <PreferencesForm />
    </>
  );
}
