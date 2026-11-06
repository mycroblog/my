// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewRole } from "../features/admin";
import { accountRolesQueryOptions, rolesQueryOptions } from "../features/moderation";
import { viewerQueryOptions } from "../features/session";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * Creating a role (FR-720), for manage_roles: the viewer's roles bound its position and permissions. The `_` keeps the
 * page out of the list's layout at `/admin/roles`.
 */
export const Route = createFileRoute("/admin/roles_/new")({
  staticData: { title: "client:admin.roles.new.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_roles", async () => {
      const viewer = await context.queryClient.query(viewerQueryOptions(context.api));
      await Promise.all([
        context.queryClient.query({ ...rolesQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.query({ ...accountRolesQueryOptions(context.api, viewer.id), staleTime: Infinity }),
      ]);
    }),
  component: NewRolePage,
  errorComponent: NewRoleError,
});

function NewRoleHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.roles.new.title")}</PageHeading>
      <Link href="/admin/roles" variant="standalone" size="sm">
        {t("client:admin.role.back")}
      </Link>
    </Stack>
  );
}

function NewRolePage() {
  return (
    <>
      <NewRoleHeading />
      <NewRole />
    </>
  );
}

function NewRoleError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_roles" heading={t("client:admin.roles.new.title")} />;
}
