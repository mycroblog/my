// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import {
  PasswordSettings,
  linkedIdentitiesQueryOptions,
  passkeysQueryOptions,
  passwordStatusQueryOptions,
} from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** Changing the password (FR-002), and removing it or setting it again with a passkey (FR-004). */
export const Route = createFileRoute("/settings/password")({
  staticData: { title: "client:settings.password.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.query({ ...passwordStatusQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...passkeysQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...linkedIdentitiesQueryOptions(context.api), staleTime: Infinity }),
    ]);
  },
  component: PasswordPage,
});

function PasswordPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.password.title")}</SettingsHeading>
      <PasswordSettings />
    </>
  );
}
