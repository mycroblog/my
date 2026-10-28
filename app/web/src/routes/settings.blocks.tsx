// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { BlockedAccounts, blocksQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** The accounts the user blocks (FR-303). */
export const Route = createFileRoute("/settings/blocks")({
  staticData: { title: "client:settings.blocks.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    context.queryClient.infiniteQuery({ ...blocksQueryOptions(context.api), staleTime: Infinity }),
  component: BlocksPage,
});

function BlocksPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.blocks.title")}</SettingsHeading>
      <BlockedAccounts />
    </>
  );
}
