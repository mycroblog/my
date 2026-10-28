// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import {
  AccountMoveSettings,
  linkedIdentitiesQueryOptions,
  passkeysQueryOptions,
  passwordStatusQueryOptions,
} from "../features/settings";
import { viewerQueryOptions } from "../features/session";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** Moving the account to another account, and cancelling the move (FR-009, ADR-0089). */
export const Route = createFileRoute("/settings/move")({
  staticData: { title: "client:account_move.move.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.query({ ...viewerQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...passwordStatusQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...passkeysQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...linkedIdentitiesQueryOptions(context.api), staleTime: Infinity }),
    ]);
  },
  component: MovePage,
});

function MovePage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:account_move.move.title")}</SettingsHeading>
      <AccountMoveSettings />
    </>
  );
}
