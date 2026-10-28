// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import {
  LinkedAccountSettings,
  linkedIdentitiesQueryOptions,
  passkeysQueryOptions,
  passwordStatusQueryOptions,
} from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** Linked accounts (FR-010): the accounts at the login providers, unlinking one, and linking one. */
export const Route = createFileRoute("/settings/linked-accounts")({
  staticData: { title: "client:settings.linked_accounts.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.query({ ...linkedIdentitiesQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...passwordStatusQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...passkeysQueryOptions(context.api), staleTime: Infinity }),
    ]);
  },
  component: LinkedAccountsPage,
});

function LinkedAccountsPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.linked_accounts.title")}</SettingsHeading>
      <LinkedAccountSettings />
    </>
  );
}
