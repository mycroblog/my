// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps, notFound } from "@tanstack/react-router";
import { z } from "zod";
import { RoleDetail } from "../features/admin";
import { accountRolesQueryOptions, rolesQueryOptions } from "../features/moderation";
import { viewerQueryOptions } from "../features/session";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** A role's key (CreateRoleRequest). The `_` keeps the page out of the list's layout at `/admin/roles`. */
const paramsSchema = z.object({ key: z.string().min(1).max(64) });

/**
 * One role (FR-720, ADR-0070): what it is, changing its position, permissions and badge, and deleting it, for
 * manage_roles. There is no GET of one role: the page reads the roles of the instance (a short list), and a key that
 * no role has is the not-found page.
 */
export const Route = createFileRoute("/admin/roles_/$key")({
  staticData: { title: "client:admin.role.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { key } }) =>
    loadWithPermission(context, "manage_roles", async () => {
      const viewer = await context.queryClient.query(viewerQueryOptions(context.api));
      const [roles] = await Promise.all([
        context.queryClient.query({ ...rolesQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.query({
          ...accountRolesQueryOptions(context.api, viewer.id),
          staleTime: Infinity,
        }),
      ]);
      if (!roles.items.some((role) => role.key === key)) notFound({ throw: true });
    }),
  component: RolePage,
  errorComponent: RoleError,
});

function RoleHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.role.title")}</PageHeading>
      <Link href="/admin/roles" variant="standalone" size="sm">
        {t("client:admin.role.back")}
      </Link>
    </Stack>
  );
}

function RolePage() {
  const { key } = Route.useParams();
  return (
    <>
      <RoleHeading />
      {/* Another role is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <RoleDetail key={key} roleKey={key} />
    </>
  );
}

function RoleError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_roles" heading={t("client:admin.role.title")} />;
}
