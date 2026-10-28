// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { AutoDeleteSettings, autoDeletePolicyQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** The automatic deletion of the user's own old posts (FR-118), with the exceptions that keep them. */
export const Route = createFileRoute("/settings/auto-delete")({
  staticData: { title: "client:settings.auto_delete.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    context.queryClient.query({ ...autoDeletePolicyQueryOptions(context.api), staleTime: Infinity }),
  component: AutoDeletePage,
});

function AutoDeletePage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.auto_delete.title")}</SettingsHeading>
      <AutoDeleteSettings />
    </>
  );
}
