// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { DomainMutes, domainMutesQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** The domains the user mutes (FR-304). */
export const Route = createFileRoute("/settings/domain-mutes")({
  staticData: { title: "client:settings.domain_mutes.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    context.queryClient.infiniteQuery({ ...domainMutesQueryOptions(context.api), staleTime: Infinity }),
  component: DomainMutesPage,
});

function DomainMutesPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.domain_mutes.title")}</SettingsHeading>
      <DomainMutes />
    </>
  );
}
