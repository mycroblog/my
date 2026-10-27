// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { linkedIdentitiesQueryOptions, PasskeySettings, passkeysQueryOptions, passwordStatusQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** Passkeys (FR-004): adding one, the list, and removing one. */
export const Route = createFileRoute("/settings/passkeys")({
  staticData: { title: "client:settings.passkeys.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.query({ ...passkeysQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...passwordStatusQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...linkedIdentitiesQueryOptions(context.api), staleTime: Infinity }),
    ]);
  },
  component: PasskeysPage,
});

function PasskeysPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.passkeys.title")}</SettingsHeading>
      <PasskeySettings />
    </>
  );
}
