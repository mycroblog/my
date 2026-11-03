// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import {
  dataExportsQueryOptions,
  deactivationPreviewQueryOptions,
  linkedIdentitiesQueryOptions,
  OwnAccount,
  passkeysQueryOptions,
  passwordStatusQueryOptions,
  twoFactorQueryOptions,
} from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** The user's own account (FR-008): the archives of the user's data, and deactivating and deleting the account. */
export const Route = createFileRoute("/settings/account")({
  staticData: { title: "client:settings.account.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.query({ ...dataExportsQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...passwordStatusQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...passkeysQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...linkedIdentitiesQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...deactivationPreviewQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...twoFactorQueryOptions(context.api), staleTime: Infinity }),
    ]);
  },
  component: AccountPage,
});

function AccountPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.account.title")}</SettingsHeading>
      <OwnAccount />
    </>
  );
}
