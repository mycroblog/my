// SPDX-License-Identifier: AGPL-3.0-only
// The roles screen below its heading (FR-720, manage_roles): the form that creates a role, then the roles, highest
// position first. Each role shows its position and its permissions by area (ADR-0058); one at or below the viewer's
// highest role is changed in a dialog (position and permissions; never admin), gets a badge in another dialog (a name
// and one of the badge colors, with a sample; admin included, ADR-0071) and is deleted after an alert dialog (never a
// built-in role, and never one that someone holds). A role above the viewer's says why it has no buttons.
// Roles are granted on an account's admin page. What a change did stays above the list in a StatusMessage; a refusal
// stays in the form or the dialog.
import { type BadgeColor, badgeColors, Button, Checkbox, Cluster, ConfirmDialog, DescriptionItem, DescriptionList, Dialog, DialogTrigger, Grid, Heading, Hint, InlineError, Link, Radio, RadioGroup, RoleBadge, SectionHeader, Stack, StatusMessage, Text, TextField } from "@my/ui";
import { type SubmitEvent, useId, useRef, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { DetailSection, type Permission, RecordList, RequirePermission } from "../../moderation";
import { adminRoleKey, type AdminRole, badgeLabelLimit, moderatorRoleKey, permissionCategories, roleKeyLimit } from "../api/roles";
import {
  type BadgeDraft,
  draftOfBadge,
  draftOfRole,
  isolatedRoleKey,
  type RoleActions,
  type RoleDraft,
  type RoleFieldError,
  type RoleRights,
  type RoleRow,
  togglePermission,
  useNewRoleForm,
  useRoleActions,
  useRoleSettings,
} from "../hooks/useRoleSettings";

const permissionLabels: Readonly<Record<Permission, ClientMessageKey>> = {
  manage_reports: "client:admin.roles.permission.manage_reports",
  manage_accounts: "client:admin.roles.permission.manage_accounts",
  manage_domains: "client:admin.roles.permission.manage_domains",
  manage_rules: "client:admin.roles.permission.manage_rules",
  manage_settings: "client:admin.roles.permission.manage_settings",
  manage_roles: "client:admin.roles.permission.manage_roles",
  manage_invites: "client:admin.roles.permission.manage_invites",
  view_audit: "client:admin.roles.permission.view_audit",
  manage_jobs: "client:admin.roles.permission.manage_jobs",
  manage_login_recovery: "client:admin.roles.permission.manage_login_recovery",
  manage_custom_emojis: "client:admin.roles.permission.manage_custom_emojis",
  manage_webhooks: "client:admin.roles.permission.manage_webhooks",
  manage_announcements: "client:admin.roles.permission.manage_announcements",
  manage_finance: "client:admin.roles.permission.manage_finance",
  view_finance: "client:admin.roles.permission.view_finance",
  manage_supporters: "client:admin.roles.permission.manage_supporters",
  create_custom_emojis: "client:admin.roles.permission.create_custom_emojis",
  manage_oauth_apps: "client:admin.roles.permission.manage_oauth_apps",
};

/** The name of each badge color. */
const badgeColorLabels: Readonly<Record<BadgeColor, ClientMessageKey>> = {
  neutral: "client:admin.roles.badge.color.neutral",
  red: "client:admin.roles.badge.color.red",
  orange: "client:admin.roles.badge.color.orange",
  yellow: "client:admin.roles.badge.color.yellow",
  green: "client:admin.roles.badge.color.green",
  teal: "client:admin.roles.badge.color.teal",
  blue: "client:admin.roles.badge.color.blue",
  purple: "client:admin.roles.badge.color.purple",
  pink: "client:admin.roles.badge.color.pink",
};

const categoryLabels: Readonly<Record<"moderation" | "server" | "member", ClientMessageKey>> = {
  moderation: "client:admin.roles.category.moderation",
  server: "client:admin.roles.category.server",
  member: "client:admin.roles.category.member",
};

/** A role's key, an identifier that reads left to right in every locale. */
function RoleKey({ value }: { value: string }) {
  return (
    <Text wrap="anywhere" dir="ltr">
      {value}
    </Text>
  );
}

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
  return (
    <Stack gap="8">
      <DetailSection title={t("client:admin.roles.new.title")}>
        <NewRoleForm rights={rights} />
      </DetailSection>
      <DetailSection title={t("client:admin.roles.list.title")}>
        <RoleList rows={rows} rights={rights} />
      </DetailSection>
    </Stack>
  );
}

function NewRoleForm({ rights }: { rights: RoleRights }) {
  const t = useT();
  const form = useNewRoleForm(rights);
  if (rights.top === undefined) return <Hint size="inherit">{t("client:admin.roles.new.no_role")}</Hint>;
  return (
    <Stack as="form" container gap="6" onSubmit={form.submit} noValidate>
      <Stack gap="1" align="start">
        <Hint>{t("client:admin.roles.new.description")}</Hint>
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
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.roles.submit")}
        </Button>
      </div>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

type RoleFieldsProps = {
  draft: RoleDraft;
  change: (change: Partial<RoleDraft>) => void;
  error: RoleFieldError | undefined;
  rights: RoleRights;
  autoFocus?: boolean;
};

/** The position and the permissions of a role, the permissions in a group per area. */
function RoleFields({ draft, change, error, rights, autoFocus = false }: RoleFieldsProps) {
  const t = useT();
  const hintId = useId();
  const errorId = useId();
  return (
    <>
      <TextField
        label={t("client:admin.roles.position")}
        description={t("client:admin.roles.position.description", { max: String(rights.maxPosition) })}
        value={draft.position}
        onChange={(position) => {
          change({ position });
        }}
        isRequired
        inputMode="numeric"
        autoComplete="off"
        autoFocus={autoFocus}
        {...(error?.field === "position" ? { isInvalid: true, errorMessage: error.message } : {})}
      />
      <Hint id={hintId}>
        {t("client:admin.roles.permissions.description")}
      </Hint>
      {permissionCategories.map(({ category, permissions }) => (
        <PermissionGroup
          key={category}
          label={t(categoryLabels[category])}
          permissions={permissions}
          draft={draft}
          change={change}
          rights={rights}
          describedBy={error?.field === "permissions" ? `${hintId} ${errorId}` : hintId}
        />
      ))}
      {error?.field === "permissions" && (
        <Stack gap="1" tone="danger" textSize="sm" id={errorId} role="alert">
          <p>{error.message}</p>
          {error.missing.length > 0 && <p>{t("client:admin.roles.missing_permissions", { permissions: `⁦${error.missing.join(", ")}⁩` })}</p>}
        </Stack>
      )}
      {error?.field === "form" && (
        <InlineError size="inherit" alert>
          {t("client:admin.roles.save_failed", { reason: error.message })}
        </InlineError>
      )}
    </>
  );
}

type PermissionGroupProps = {
  label: string;
  permissions: readonly Permission[];
  draft: RoleDraft;
  change: (change: Partial<RoleDraft>) => void;
  rights: RoleRights;
  describedBy: string;
};

/** The permissions of one area; those the viewer's roles do not grant cannot be given or taken. */
function PermissionGroup({ label, permissions, draft, change, rights, describedBy }: PermissionGroupProps) {
  const t = useT();
  const labelId = useId();
  return (
    <Stack gap="2" role="group" aria-labelledby={labelId} aria-describedby={describedBy}>
      <Text size="sm" tone="fg" weight="medium" id={labelId}>
        {label}
      </Text>
      <Grid at="md" gap="2">
        {permissions.map((permission) => (
          <Checkbox
            key={permission}
            isSelected={draft.permissions.includes(permission)}
            isDisabled={!rights.permissions.includes(permission)}
            onChange={(selected) => {
              change({ permissions: togglePermission(draft.permissions, permission, selected) });
            }}
          >
            {t(permissionLabels[permission])}
          </Checkbox>
        ))}
      </Grid>
    </Stack>
  );
}

function RoleList({ rows, rights }: { rows: RoleRow[]; rights: RoleRights }) {
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    rows.map((row) => row.role.id),
  );
  const actions = useRoleActions(rights, removed);
  const list = { items: rows, hasMore: false, isLoadingMore: false, loadMore: () => undefined };
  return (
    <Stack gap="3">
      <StatusMessage>{actions.status}</StatusMessage>
      <RecordList list={list} getId={(row) => row.role.id} listRef={listRef} empty={null}>
        {(row) => <RoleItem row={row} rights={rights} actions={actions} />}
      </RecordList>
    </Stack>
  );
}

/** The permissions of a role in one area, as a sentence. */
function permissionNames(role: AdminRole, permissions: readonly Permission[], t: ReturnType<typeof useT>): string {
  const names = permissions.filter((permission) => role.permissions.includes(permission)).map((permission) => t(permissionLabels[permission]));
  return names.length === 0 ? t("client:admin.roles.permissions.none") : names.join(t("client:admin.roles.permissions.separator"));
}

function RoleItem({ row, rights, actions }: { row: RoleRow; rights: RoleRights; actions: RoleActions }) {
  const t = useT();
  const { role, outrankedBy } = row;
  const builtin =
    role.key === adminRoleKey
      ? t("client:admin.roles.builtin.admin")
      : role.key === moderatorRoleKey
        ? t("client:admin.roles.builtin.moderator")
        : undefined;
  return (
    <Stack gap="3">
      <SectionHeader
        actions={
          (row.canChange || row.canChangeBadge || row.canDelete) && (
            <>
              {row.canChange && <EditRole role={role} rights={rights} actions={actions} />}
              {row.canChangeBadge && <EditBadge role={role} actions={actions} />}
              {row.canDelete && <DeleteRole role={role} actions={actions} />}
            </>
          )
        }
      >
        <Heading level={3}>
          <RoleKey value={role.key} />
        </Heading>
      </SectionHeader>
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.roles.position")}>{role.position}</DescriptionItem>
        {permissionCategories.map(({ category, permissions }) => (
          <DescriptionItem key={category} term={t(categoryLabels[category])}>
            {permissionNames(role, permissions, t)}
          </DescriptionItem>
        ))}
        <DescriptionItem term={t("client:admin.roles.badge")}>
          {role.badge === undefined ? (
            t("client:admin.roles.badge.none")
          ) : (
            <RoleBadge label={role.badge.label} color={role.badge.color} />
          )}
        </DescriptionItem>
      </DescriptionList>
      {builtin !== undefined && <Hint>{builtin}</Hint>}
      {outrankedBy !== undefined && (
        <Hint>{t("client:admin.roles.outranked", { role: isolatedRoleKey(outrankedBy.key) })}</Hint>
      )}
    </Stack>
  );
}

/** The position and permissions of a role in a dialog; it closes once they are saved, and keeps a refusal on its field. */
function EditRole({ role, rights, actions }: { role: AdminRole; rights: RoleRights; actions: RoleActions }) {
  const t = useT();
  const key = isolatedRoleKey(role.key);
  return (
    <DialogTrigger>
      <Button size="sm" aria-label={t("client:admin.roles.edit_role", { role: key })}>
        {t("client:admin.roles.edit")}
      </Button>
      <Dialog title={t("client:admin.roles.edit.title", { role: key })} closeLabel={t("client:admin.roles.edit.close")} isDismissable={false} size="md">
        {({ close }) => <EditRoleForm role={role} rights={rights} actions={actions} close={close} />}
      </Dialog>
    </DialogTrigger>
  );
}

function EditRoleForm({ role, rights, actions, close }: { role: AdminRole; rights: RoleRights; actions: RoleActions; close: () => void }) {
  const t = useT();
  const [draft, setDraft] = useState<RoleDraft>(() => draftOfRole(role));
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<RoleFieldError | undefined>(undefined);
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    setIsSaving(true);
    setError(undefined);
    void actions.update(role, draft).then((refused) => {
      setIsSaving(false);
      if (refused === undefined) close();
      else setError(refused);
    });
  };
  return (
    <Stack as="form" container gap="4" onSubmit={onSubmit} noValidate>
      <RoleFields
        draft={draft}
        change={(change) => {
          setDraft((previous) => ({ ...previous, ...change }));
        }}
        error={error}
        rights={rights}
        autoFocus
      />
      <Cluster gap="2" justify="end">
        <Button onPress={close}>{t("client:admin.roles.cancel")}</Button>
        <Button type="submit" variant="primary" isPending={isSaving}>
          {t("client:admin.roles.save")}
        </Button>
      </Cluster>
    </Stack>
  );
}

/** The badge of a role in a dialog: its name and color with a sample, saved or removed; it closes once that is done. */
function EditBadge({ role, actions }: { role: AdminRole; actions: RoleActions }) {
  const t = useT();
  const key = isolatedRoleKey(role.key);
  return (
    <DialogTrigger>
      <Button size="sm" aria-label={t("client:admin.roles.badge.edit_role", { role: key })}>
        {t("client:admin.roles.badge.edit")}
      </Button>
      <Dialog title={t("client:admin.roles.badge.edit.title", { role: key })} closeLabel={t("client:admin.roles.edit.close")} isDismissable={false} size="md">
        {({ close }) => <EditBadgeForm role={role} actions={actions} close={close} />}
      </Dialog>
    </DialogTrigger>
  );
}

function EditBadgeForm({ role, actions, close }: { role: AdminRole; actions: RoleActions; close: () => void }) {
  const t = useT();
  const [draft, setDraft] = useState<BadgeDraft>(() => draftOfBadge(role));
  const [pending, setPending] = useState<"save" | "remove" | undefined>(undefined);
  const [error, setError] = useState<RoleFieldError | undefined>(undefined);
  const send = (next: BadgeDraft | null) => {
    setPending(next === null ? "remove" : "save");
    setError(undefined);
    void actions.updateBadge(role, next).then((refused) => {
      setPending(undefined);
      if (refused === undefined) close();
      else setError(refused);
    });
  };
  const sample = draft.label.trim();
  return (
    <Stack as="form" container gap="4"
      onSubmit={(event: SubmitEvent<HTMLFormElement>) => {
        event.preventDefault();
        send(draft);
      }}
      noValidate>
      <Hint>{t("client:admin.roles.badge.description")}</Hint>
      <TextField
        label={t("client:admin.roles.badge.label")}
        description={t("client:admin.roles.badge.label.description", { max: String(badgeLabelLimit) })}
        value={draft.label}
        onChange={(label) => {
          setDraft((previous) => ({ ...previous, label }));
        }}
        isRequired
        autoComplete="off"
        autoFocus
        {...(error?.field === "badge" ? { isInvalid: true, errorMessage: error.message } : {})}
      />
      <RadioGroup
        label={t("client:admin.roles.badge.color")}
        description={t("client:admin.roles.badge.color.description")}
        orientation="horizontal"
        value={draft.color}
        onChange={(color) => {
          const chosen = badgeColors.find((c) => c === color);
          if (chosen !== undefined) setDraft((previous) => ({ ...previous, color: chosen }));
        }}
      >
        {badgeColors.map((color) => (
          <Radio key={color} value={color}>
            {t(badgeColorLabels[color])}
          </Radio>
        ))}
      </RadioGroup>
      <Stack gap="1" align="start">
        <Text size="sm" tone="fg" weight="medium">{t("client:admin.roles.badge.sample")}</Text>
        {sample === "" ? (
          <Hint as="span">{t("client:admin.roles.badge.sample.empty")}</Hint>
        ) : (
          <RoleBadge label={sample} color={draft.color} />
        )}
      </Stack>
      {error?.field === "form" && (
        <InlineError size="inherit" alert>
          {t("client:admin.roles.save_failed", { reason: error.message })}
        </InlineError>
      )}
      <Cluster gap="2" justify="end">
        {role.badge !== undefined && (
          <Button
            variant="danger"
            isPending={pending === "remove"}
            isDisabled={pending === "save"}
            onPress={() => {
              send(null);
            }}
          >
            {t("client:admin.roles.badge.remove")}
          </Button>
        )}
        <Button onPress={close}>{t("client:admin.roles.cancel")}</Button>
        <Button type="submit" variant="primary" isPending={pending === "save"} isDisabled={pending === "remove"}>
          {t("client:admin.roles.save")}
        </Button>
      </Cluster>
    </Stack>
  );
}

/** Deleting a role, after an alert dialog: it cannot be undone, and the server refuses a role that someone holds. */
function DeleteRole({ role, actions }: { role: AdminRole; actions: RoleActions }) {
  const t = useT();
  const key = isolatedRoleKey(role.key);
  const [isDeleting, setIsDeleting] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  return (
    <DialogTrigger
      onOpenChange={() => {
        setError(undefined);
      }}
    >
      <Button size="sm" variant="danger" aria-label={t("client:admin.roles.delete_role", { role: key })}>
        {t("client:admin.roles.delete")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.roles.delete.title", { role: key })}
        description={t("client:admin.roles.delete.description")}
        confirmLabel={t("client:admin.roles.delete")}
        cancelLabel={t("client:admin.roles.cancel")}
        isPending={isDeleting}
        {...(error !== undefined && { errorMessage: t("client:admin.roles.delete_failed", { reason: error }) })}
        onConfirm={(close) => {
          setIsDeleting(true);
          setError(undefined);
          void actions.remove(role).then((failed) => {
            setIsDeleting(false);
            if (failed === undefined) close();
            else setError(failed);
          });
        }}
      />
    </DialogTrigger>
  );
}
