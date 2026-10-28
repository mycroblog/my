// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { OwnInvites, ownInvitesQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/**
 * The invites the user created (FR-011, ADR-0108 §5): creating one within the caps of the instance settings, listing
 * them, copying a link, and revoking one. The settings list the page only for a user who may create invites now; a
 * user who no longer may still opens it from its address to list and revoke theirs.
 */
export const Route = createFileRoute("/settings/invites")({
  staticData: { title: "client:settings.invites.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    context.queryClient.infiniteQuery({ ...ownInvitesQueryOptions(context.api), staleTime: Infinity }),
  component: InvitesPage,
});

function InvitesPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.invites.title")}</SettingsHeading>
      <OwnInvites />
    </>
  );
}
