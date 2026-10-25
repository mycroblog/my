// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { AliasSettings, aliasesQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** The accounts the user moves from (FR-009, ADR-0089): listing, adding, and removing the aliases. */
export const Route = createFileRoute("/settings/aliases")({
  staticData: { title: "client:account_move.aliases.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => context.queryClient.query({ ...aliasesQueryOptions(context.api), staleTime: Infinity }),
  component: AliasesPage,
});

function AliasesPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:account_move.aliases.title")}</SettingsHeading>
      <AliasSettings />
    </>
  );
}
