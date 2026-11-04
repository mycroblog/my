// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { BrowserSessions, browserSessionsQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** Signed-in browsers (ADR-0090): where the account is signed in, and signing out one browser or the others. */
export const Route = createFileRoute("/settings/sessions")({
  staticData: { title: "client:settings.sessions.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context }) => {
    await context.queryClient.query({ ...browserSessionsQueryOptions(context.api), staleTime: Infinity });
  },
  component: BrowserSessionsPage,
});

function BrowserSessionsPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.sessions.title")}</SettingsHeading>
      <BrowserSessions />
    </>
  );
}
