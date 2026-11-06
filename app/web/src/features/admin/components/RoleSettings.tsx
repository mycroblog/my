// SPDX-License-Identifier: AGPL-3.0-only
// The roles screen below its heading (FR-720, manage_roles): "Create a role" (a link to its page), then the roles,
// highest position first, in a table of four columns: the key (a link to the role's page, where it is changed and
// deleted), the position, how many permissions it grants in each area (ADR-0058), and its badge (ADR-0070). Roles are
// granted on an account's admin page. A viewer who holds no role cannot create one, and the page says so.
import { ButtonLink, EmptyState, Hint, Link, Stack, Text } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { type RecordColumns, RecordTable, RequirePermission, wholeList } from "../../moderation";
import { permissionCategories } from "../api/roles";
import { type RoleRow, rolePath, useRoleSettings } from "../hooks/useRoleSettings";
import { categoryLabels, RoleBadgeValue, RoleKey } from "./roleParts";

export function RoleSettings() {
  return (
    <RequirePermission permission="manage_roles">
      <RoleScreen />
    </RequirePermission>
  );
}

function RoleScreen() {
  const t = useT();
  const { rows, rights } = useRoleSettings();
  const columns: RecordColumns<RoleRow> = [
    {
      id: "key",
      header: t("client:admin.roles.key"),
      wrap: "anywhere",
      cell: ({ role }) => (
        <Link href={rolePath(role.key)} variant="standalone" wrap="anywhere">
          <RoleKey value={role.key} />
        </Link>
      ),
    },
    {
      id: "position",
      header: t("client:admin.roles.position"),
      numeric: true,
      cell: ({ role }) => String(role.position),
    },
    {
      id: "permissions",
      header: t("client:admin.roles.permissions"),
      cell: ({ role }) => (
        <Stack gap="0">
          {permissionCategories.map(({ category, permissions }) => (
            <Text key={category}>
              {t("client:admin.roles.permission_count", {
                area: t(categoryLabels[category]),
                count: permissions.filter((permission) => role.permissions.includes(permission)).length,
              })}
            </Text>
          ))}
        </Stack>
      ),
    },
    { id: "badge", header: t("client:admin.roles.badge"), cell: ({ role }) => <RoleBadgeValue role={role} /> },
  ];
  return (
    <Stack gap="6">
      <Stack gap="3" align="start">
        <Hint size="inherit">{t("client:admin.roles.description")}</Hint>
        {rights.top === undefined ? (
          <Hint size="inherit">{t("client:admin.roles.new.no_role")}</Hint>
        ) : (
          <ButtonLink href="/admin/roles/new" variant="primary">
            {t("client:admin.roles.new.title")}
          </ButtonLink>
        )}
      </Stack>
      <RecordTable
        list={wholeList(rows)}
        caption={t("client:admin.roles.list.title")}
        columns={columns}
        getId={({ role }) => role.id}
        headingLevel={2}
        empty={<EmptyState headingLevel={2} heading={t("client:admin.roles.empty")} />}
      />
    </Stack>
  );
}
