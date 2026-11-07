// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { Invites, invitesQueryOptions, moderationInstanceQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { notFoundOnSingleUser } from "../lib/singleUser";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The list of the invites that let people sign up (FR-001): copying and revoking them, for manage_invites. */
export const Route = createFileRoute("/moderation/invites")({
  staticData: { title: "client:admin.invites.title" },
  // A one-person server has none of this (ADR-0152 §4): the address is not found there.
  beforeLoad: async (match) => {
    await requireSignIn(match);
    await notFoundOnSingleUser(match);
  },
  loader: ({ context }) =>
    loadWithPermission(context, "manage_invites", () =>
      Promise.all([
        context.queryClient.infiniteQuery({ ...invitesQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.query({ ...moderationInstanceQueryOptions(context.api), staleTime: Infinity }),
      ]),
    ),
  component: InvitesPage,
  errorComponent: InvitesError,
});

function InvitesPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.invites.title")}</PageHeading>
      <Invites />
    </>
  );
}

function InvitesError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_invites" heading={t("client:admin.invites.title")} />;
}
