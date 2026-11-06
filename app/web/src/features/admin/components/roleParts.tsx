// SPDX-License-Identifier: AGPL-3.0-only
// What the pages of the roles share (FR-720, ADR-0058, ADR-0070): the names of the permissions, their areas and the
// badge colors, a role's key, the position and permissions of a role (the permissions in a group per area; those the
// viewer's roles do not grant cannot be given or taken), the name and color of its badge with a sample, and a role's
// permissions in one area as a sentence.
import {
  type BadgeColor,
  badgeColors,
  Checkbox,
  Grid,
  Hint,
  Radio,
  RadioGroup,
  RoleBadge,
  Stack,
  Text,
  TextField,
} from "@my/ui";
import { useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { Permission } from "../../moderation";
import { type AdminRole, badgeLabelLimit, permissionCategories } from "../api/roles";
import {
  type BadgeDraft,
  type RoleDraft,
  type RoleFieldError,
  type RoleRights,
  togglePermission,
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
  manage_service_closure: "client:admin.roles.permission.manage_service_closure",
  start_voice_rooms: "client:admin.roles.permission.start_voice_rooms",
  create_invites: "client:admin.roles.permission.create_invites",
  manage_errors: "client:admin.roles.permission.manage_errors",
  manage_login_providers: "client:admin.roles.permission.manage_login_providers",
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

export const categoryLabels: Readonly<Record<"moderation" | "server" | "member", ClientMessageKey>> = {
  moderation: "client:admin.roles.category.moderation",
  server: "client:admin.roles.category.server",
  member: "client:admin.roles.category.member",
};

/** A role's key, an identifier that reads left to right in every locale. */
export function RoleKey({ value }: { value: string }) {
  return (
    <Text wrap="anywhere" dir="ltr">
      {value}
    </Text>
  );
}

type RoleFieldsProps = {
  draft: RoleDraft;
  change: (change: Partial<RoleDraft>) => void;
  error: RoleFieldError | undefined;
  rights: RoleRights;
};

/** The position and the permissions of a role, the permissions in a group per area. */
export function RoleFields({ draft, change, error, rights }: RoleFieldsProps) {
  const t = useT();
  const hintId = useId();
  const errorId = useId();
  return (
    <>
      <TextField
        label={t("client:admin.roles.position")}
        description={t("client:admin.roles.position.description", {
          max: String(rights.maxPosition),
        })}
        value={draft.position}
        onChange={(position) => {
          change({ position });
        }}
        isRequired
        inputMode="numeric"
        autoComplete="off"
        {...(error?.field === "position" ? { isInvalid: true, errorMessage: error.message } : {})}
      />
      <Hint id={hintId}>{t("client:admin.roles.permissions.description")}</Hint>
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
          {error.missing.length > 0 && (
            <p>
              {t("client:admin.roles.missing_permissions", {
                permissions: `⁦${error.missing.join(", ")}⁩`,
              })}
            </p>
          )}
        </Stack>
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

type BadgeFieldsProps = {
  draft: BadgeDraft;
  change: (change: Partial<BadgeDraft>) => void;
  error: RoleFieldError | undefined;
};

/** The name and color of a role's badge, with a sample of it as typed; no name means no badge. */
export function BadgeFields({ draft, change, error }: BadgeFieldsProps) {
  const t = useT();
  const labelId = useId();
  const sample = draft.label.trim();
  return (
    <Stack gap="4" role="group" aria-labelledby={labelId}>
      <Stack gap="1">
        <Text size="sm" tone="fg" weight="medium" id={labelId}>
          {t("client:admin.roles.badge")}
        </Text>
        <Hint>{t("client:admin.roles.badge.description")}</Hint>
      </Stack>
      <TextField
        label={t("client:admin.roles.badge.label")}
        description={t("client:admin.roles.badge.label.description", {
          max: String(badgeLabelLimit),
        })}
        value={draft.label}
        onChange={(label) => {
          change({ label });
        }}
        autoComplete="off"
        {...(error?.field === "badge" ? { isInvalid: true, errorMessage: error.message } : {})}
      />
      <RadioGroup
        label={t("client:admin.roles.badge.color")}
        description={t("client:admin.roles.badge.color.description")}
        orientation="horizontal"
        value={draft.color}
        onChange={(color) => {
          const chosen = badgeColors.find((c) => c === color);
          if (chosen !== undefined) change({ color: chosen });
        }}
      >
        {badgeColors.map((color) => (
          <Radio key={color} value={color}>
            {t(badgeColorLabels[color])}
          </Radio>
        ))}
      </RadioGroup>
      <Stack gap="1" align="start">
        <Text size="sm" tone="fg" weight="medium">
          {t("client:admin.roles.badge.sample")}
        </Text>
        {sample === "" ? (
          <Hint as="span">{t("client:admin.roles.badge.sample.empty")}</Hint>
        ) : (
          <RoleBadge label={sample} color={draft.color} />
        )}
      </Stack>
    </Stack>
  );
}

/** The permissions of a role in one area, as a sentence. */
export function permissionNames(
  role: AdminRole,
  permissions: readonly Permission[],
  t: ReturnType<typeof useT>,
): string {
  const names = permissions
    .filter((permission) => role.permissions.includes(permission))
    .map((permission) => t(permissionLabels[permission]));
  return names.length === 0
    ? t("client:admin.roles.permissions.none")
    : names.join(t("client:admin.roles.permissions.separator"));
}

/** A role's badge, or that it has none. */
export function RoleBadgeValue({ role }: { role: AdminRole }) {
  const t = useT();
  return role.badge === undefined ? (
    t("client:admin.roles.badge.none")
  ) : (
    <RoleBadge label={role.badge.label} color={role.badge.color} />
  );
}
