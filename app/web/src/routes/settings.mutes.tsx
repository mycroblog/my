// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { MutedAccounts, mutesQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** The accounts the user mutes (FR-303). */
export const Route = createFileRoute("/settings/mutes")({
  staticData: { title: "client:settings.mutes.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    context.queryClient.infiniteQuery({ ...mutesQueryOptions(context.api), staleTime: Infinity }),
  component: MutesPage,
});

function MutesPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.mutes.title")}</SettingsHeading>
      <MutedAccounts />
    </>
  );
}
