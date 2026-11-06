// SPDX-License-Identifier: AGPL-3.0-only
// The page of one role below its heading (FR-720, manage_roles): what it is (its position, its permissions by area
// (ADR-0058) and its badge (ADR-0070)); then, for a role at or below the viewer's highest one, one form with one save:
// its position and permissions (never for admin) and the name and color of its badge with a sample (no name for no
// badge); and deleting it after an alert dialog (never a built-in role, and never one that someone holds), which opens
// the list of roles with a toast. A role above the viewer's says why it cannot be changed. Roles are granted on an
// account's admin page: the API lists no accounts by role, so the page links to the account search.
import {
  Button,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  FormActionBar,
  Hint,
  Link,
  Stack,
} from "@my/ui";
import { useState } from "react";
import { useT } from "../../../lib/i18n";
import { RequirePermission, Section } from "../../moderation";
import { adminRoleKey, type AdminRole, moderatorRoleKey, permissionCategories } from "../api/roles";
import {
  isolatedRoleKey,
  type RoleRights,
  type RoleRow,
  useDeleteRole,
  useRole,
  useRoleForm,
} from "../hooks/useRoleSettings";
import { BadgeFields, categoryLabels, permissionNames, RoleBadgeValue, RoleFields, RoleKey } from "./roleParts";

export function RoleDetail({ roleKey }: { roleKey: string }) {
  return (
    <RequirePermission permission="manage_roles">
      <RoleScreen roleKey={roleKey} />
    </RequirePermission>
  );
}

function RoleScreen({ roleKey }: { roleKey: string }) {
  const t = useT();
  const { row, rights } = useRole(roleKey);
  // The route's loader sends an unknown key to the not-found page; a role deleted meanwhile leaves this.
  if (row === undefined) return <Hint size="inherit">{t("client:admin.role.gone")}</Hint>;
  return (
    <Stack gap="8">
      <Section surface="card" title={t("client:admin.role.facts")}>
        <RoleFacts row={row} />
      </Section>
      {row.canChangeBadge && (
        <Section surface="card" title={t("client:admin.role.edit.title")}>
          <RoleForm row={row} rights={rights} />
        </Section>
      )}
      {row.canDelete && (
        <Section
          surface="card"
          title={t("client:admin.role.delete.title")}
          description={t("client:admin.roles.delete.description")}
        >
          <DeleteRole role={row.role} />
        </Section>
      )}
    </Stack>
  );
}

function RoleFacts({ row }: { row: RoleRow }) {
  const t = useT();
  const { role, outrankedBy } = row;
  const builtin =
    role.key === adminRoleKey
      ? t("client:admin.roles.builtin.admin")
      : role.key === moderatorRoleKey
        ? t("client:admin.roles.builtin.moderator")
        : undefined;
  return (
    <Stack gap="4">
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.roles.key")}>
          <RoleKey value={role.key} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.roles.position")}>{String(role.position)}</DescriptionItem>
        {permissionCategories.map(({ category, permissions }) => (
          <DescriptionItem key={category} term={t(categoryLabels[category])}>
            {permissionNames(role, permissions, t)}
          </DescriptionItem>
        ))}
        <DescriptionItem term={t("client:admin.roles.badge")}>
          <RoleBadgeValue role={role} />
        </DescriptionItem>
      </DescriptionList>
      {builtin !== undefined && <Hint>{builtin}</Hint>}
      {outrankedBy !== undefined && (
        <Hint>{t("client:admin.roles.outranked", { role: isolatedRoleKey(outrankedBy.key) })}</Hint>
      )}
      <Stack gap="1" align="start">
        <Hint>{t("client:admin.role.holders")}</Hint>
        <Link href="/moderation/accounts" variant="standalone">
          {t("client:admin.roles.new.accounts")}
        </Link>
      </Stack>
    </Stack>
  );
}

function RoleForm({ row, rights }: { row: RoleRow; rights: RoleRights }) {
  const t = useT();
  const form = useRoleForm(row, rights);
  return (
    <Stack as="form" container gap="6" onSubmit={form.submit} noValidate>
      {row.canChange && <RoleFields draft={form.draft} change={form.change} error={form.error} rights={rights} />}
      <BadgeFields draft={form.draft.badge} change={form.changeBadge} error={form.error} />
      <FormActionBar result={form.result}>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.roles.save")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}

/** Deleting the role, after an alert dialog: it cannot be undone, and the server refuses a role that someone holds. */
function DeleteRole({ role }: { role: AdminRole }) {
  const t = useT();
  const key = isolatedRoleKey(role.key);
  const deletion = useDeleteRole(role);
  const [error, setError] = useState<string | undefined>(undefined);
  return (
    <DialogTrigger
      onOpenChange={() => {
        setError(undefined);
      }}
    >
      <Button variant="danger">{t("client:admin.roles.delete")}</Button>
      <ConfirmDialog
        title={t("client:admin.roles.delete.title", { role: key })}
        description={t("client:admin.roles.delete.description")}
        confirmLabel={t("client:admin.roles.delete")}
        cancelLabel={t("client:admin.roles.cancel")}
        isPending={deletion.isPending}
        {...(error !== undefined && { errorMessage: t("client:admin.roles.delete_failed", { reason: error }) })}
        onConfirm={(close) => {
          setError(undefined);
          void deletion.remove().then((failed) => {
            if (failed === undefined) close();
            else setError(failed);
          });
        }}
      />
    </DialogTrigger>
  );
}
