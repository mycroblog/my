// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { moderationInstanceQueryOptions, NewInvite } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { notFoundOnSingleUser } from "../lib/singleUser";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The form of a new invite and its sign-up link (FR-001), for manage_invites. The `_` keeps it out of the list's layout. */
export const Route = createFileRoute("/moderation/invites_/new")({
  staticData: { title: "client:admin.invites.new.title" },
  // A one-person server has none of this (ADR-0152 §4): the address is not found there.
  beforeLoad: async (match) => {
    await requireSignIn(match);
    await notFoundOnSingleUser(match);
  },
  loader: ({ context }) =>
    loadWithPermission(context, "manage_invites", () =>
      context.queryClient.query({ ...moderationInstanceQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: NewInvitePage,
  errorComponent: NewInviteError,
});

function NewInvitePage() {
  const t = useT();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.invites.new.title")}</PageHeading>
        <Link href="/moderation/invites" variant="standalone" size="sm">
          {t("client:admin.invites.back")}
        </Link>
      </Stack>
      <NewInvite />
    </>
  );
}

function NewInviteError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_invites" heading={t("client:admin.invites.new.title")} />;
}
