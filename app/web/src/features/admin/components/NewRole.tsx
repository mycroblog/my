// SPDX-License-Identifier: AGPL-3.0-only
// The page of a new role below its heading (FR-720, manage_roles): its key, position and permissions, created with one
// button. Once it is created, the role's page opens (where its badge is set) and a toast says so. A refusal stays next
// to its field, or beside the button (FormActionBar), with the draft. A new role is held by nobody: it is granted on an
// account's admin page, which the page links to. A viewer who holds no role cannot create one, and the page says so.
import { Button, FormActionBar, Hint, Link, Stack, TextField } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { RequirePermission } from "../../moderation";
import { roleKeyLimit } from "../api/roles";
import { type RoleRights, useNewRoleForm, useRoleSettings } from "../hooks/useRoleSettings";
import { RoleFields } from "./roleParts";

export function NewRole() {
  return (
    <RequirePermission permission="manage_roles">
      <NewRoleScreen />
    </RequirePermission>
  );
}

function NewRoleScreen() {
  const t = useT();
  const { rights } = useRoleSettings();
  if (rights.top === undefined) return <Hint size="inherit">{t("client:admin.roles.new.no_role")}</Hint>;
  return <NewRoleForm rights={rights} />;
}

function NewRoleForm({ rights }: { rights: RoleRights }) {
  const t = useT();
  const form = useNewRoleForm(rights);
  return (
    <Stack as="form" container gap="6" onSubmit={form.submit} noValidate>
      <Stack gap="1" align="start">
        <Hint size="inherit">{t("client:admin.roles.new.description")}</Hint>
        <Link href="/moderation/accounts" variant="standalone">
          {t("client:admin.roles.new.accounts")}
        </Link>
      </Stack>
      <TextField
        label={t("client:admin.roles.key")}
        description={t("client:admin.roles.key.description", { max: String(roleKeyLimit) })}
        value={form.draft.key}
        onChange={(key) => {
          form.change({ key });
        }}
        isRequired
        maxLength={roleKeyLimit}
        autoComplete="off"
        spellCheck="false"
        {...(form.error?.field === "key" ? { isInvalid: true, errorMessage: form.error.message } : {})}
      />
      <RoleFields draft={form.draft} change={form.change} error={form.error} rights={rights} />
      <FormActionBar result={form.result}>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.roles.submit")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}
