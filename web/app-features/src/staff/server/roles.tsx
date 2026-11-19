// SPDX-License-Identifier: AGPL-3.0-only

// The roles as a holder of `manage_roles` keeps them (FR-720, ADR-0070 §8, ADR-0058 §1): at /admin/roles, every role,
// highest first, each a door to its page, with New role; a role above the reader's highest is muted and opens on what
// it is, nothing to change. A role's page, /admin/roles/{key}, is a form saved as a whole of its place, its permissions
// and its badge, and, last in a section of its own, Delete, which asks the role's name typed first. What FR-720 forbids
// is never offered: a place above the reader's highest role, a permission the reader lacks (one the role holds is said
// in a sentence, kept), anything but the badge of `admin`, the deletion of `admin` and `moderator`, and the place and
// permissions of a plan's role, which its plan sets. A new role, /admin/roles/new, is its key, its place and its
// permissions. What the server refuses of a field stands under it, which takes the focus.

import {
  badgeColors,
  builtinRoles,
  fixedPermissions,
  highestPlace,
  mayDelete,
  mayReshape,
  isStaffRole,
  lowestStaffPlace,
  offeredPermissionsAt,
  permissionOrder,
  roleLimits,
  roleRefusal,
  roleManagedByPlan,
  useAdminRole,
  useAdminRoles,
  useAdminRoleWrites,
  validRoleKey,
  withinReach,
  type AdminRole,
  type BadgeColor,
  type Permission,
  type RoleChange,
  type RoleRights,
} from "@my/app-data";
import {
  Badge,
  Box,
  Button,
  CheckField,
  ConfirmDialog,
  focusRefusedField,
  FormAlert,
  LinkButton,
  ModalPresence,
  NumberField,
  SaveState,
  Select,
  SettingRow,
  SettingsFacts,
  SettingsList,
  SettingsListRow,
  SettingsNote,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
} from "@my/design-system";
import { ProblemError } from "@mycroblog/mykit";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useStaffStrings, type T } from "../parts";
import { Irreversible, LeaveDialog, ScreenNotice, type AdminSubProps } from "../settings/parts";
import { BackTo, useStillThere, useUnsavedReport } from "./parts";

/** The address of the roles. */
export const adminRolesPath = "/admin/roles";
/** The address of a new role's form. */
const newRolePath = `${adminRolesPath}/new`;
/** The address of the role of `key`. */
export const adminRolePath = (key: string): string => `${adminRolesPath}/${encodeURIComponent(key)}`;

/** A permission's name, in the reader's language. */
export const permissionName = (t: T, permission: Permission): string => t("staff_roles.permission", { permission });

/** Names `permissions` as the reader's language joins a list. */
function permissionList(t: T, locale: string, permissions: readonly Permission[]): string {
  return new Intl.ListFormat(locale, { type: "conjunction" }).format(permissions.map((p) => permissionName(t, p)));
}

/** Why the reader may not change a role's place and permissions, or undefined where they may. */
function fixedReason(t: T, role: AdminRole, rights: RoleRights): string | undefined {
  if (!withinReach(role, rights)) return t("staff_roles.fixed.above", { key: rights.topKey ?? "" });
  if (role.key === "admin") return t("staff_roles.fixed.admin");
  if (rights.planRoles.has(role.id)) return t("staff_roles.fixed.plan");
  return undefined;
}

/** What a row of the roles says of one: its place, how many permissions it grants, its badge. */
function roleFacts(t: T, role: AdminRole): string[] {
  return [
    t("staff_roles.row.place", { position: role.position }),
    t("staff_roles.row.permissions", { count: role.permissions.length }),
    ...(role.badge === undefined ? [] : [t("staff_roles.row.badge", { label: role.badge.label })]),
  ];
}

/** A word on where a role stands for the reader: above their reach, a plan's, or built in; none for any other. */
function roleState(t: T, role: AdminRole, rights: RoleRights): string | undefined {
  if (!withinReach(role, rights)) return t("staff_roles.state.above");
  if (rights.planRoles.has(role.id)) return t("staff_roles.state.plan");
  if (builtinRoles.has(role.key)) return t("staff_roles.state.builtin");
  return undefined;
}

/** The roles: every one, highest first, a door each, and New role. */
export function AdminRolesScreen(): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminRoles();
  const title = t("staff_roles.entry");
  if (read.roles === undefined || read.rights === undefined)
    return (
      <SettingsScreen
        sub
        icon="members"
        title={title}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("staff_roles.loading"), failed: t("staff_roles.failed") }}
          />
        }
      />
    );
  const { roles, rights } = read;
  return (
    <SettingsScreen sub icon="members" title={title}>
      <SettingsSection
        heading={t("staff_roles.list.count", { count: roles.length })}
        about={
          rights.topKey === undefined
            ? undefined
            : t("staff_roles.list.about", { key: rights.topKey, position: rights.top })
        }
        action={
          <LinkButton size="timeline" href={newRolePath}>
            {t("staff_roles.add")}
          </LinkButton>
        }
      >
        <SettingsList label={t("staff_roles.entry")}>
          {roles.map((role) => (
            <SettingsListRow
              key={role.id}
              title={role.key}
              inactive={!withinReach(role, rights)}
              facts={roleFacts(t, role)}
              state={roleState(t, role, rights)}
              href={adminRolePath(role.key)}
            />
          ))}
        </SettingsList>
      </SettingsSection>
    </SettingsScreen>
  );
}

/** What the form of a role holds: its place, its permissions, and its badge (an empty label for none). */
type RoleForm = {
  readonly key: string;
  readonly position: number | null;
  readonly permissions: ReadonlySet<Permission>;
  readonly label: string;
  readonly color: BadgeColor;
};

/** The fields a refusal is said under. */
type Field = "key" | "position" | "permissions" | "label" | "color";

/** Where a role's last save is: refused, with what is wrong with which field or why in the submit bar. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" }
  | { readonly kind: "refused"; readonly field?: Field | undefined; readonly why: string };

/** Whether two sets of permissions hold the same ones. */
const samePermissions = (a: ReadonlySet<Permission>, b: ReadonlySet<Permission>) =>
  a.size === b.size && [...a].every((p) => b.has(p));

/** The field the server's refusal is about, and what to say there; else what to say in the submit bar. */
function refusalOf(t: T, locale: string, error: unknown, form: RoleForm): { field?: Field; why: string } {
  const reason = roleRefusal(error);
  switch (reason) {
    case "key":
      return { field: "key", why: t("staff_roles.key.rule") };
    case "key_taken":
      return { field: "key", why: t("staff_roles.key.taken", { key: form.key }) };
    case "position":
      return { field: "position", why: t("staff_roles.position.rule", { max: roleLimits.position }) };
    case "permissions":
    case "missing_permission": {
      const missing =
        error instanceof ProblemError && Array.isArray(error.problem["missing_permissions"])
          ? (error.problem["missing_permissions"] as Permission[])
          : [];
      return {
        field: "permissions",
        why:
          missing.length === 0
            ? t("staff_roles.permissions.refused")
            : t("staff_roles.permissions.missing", { names: permissionList(t, locale, missing) }),
      };
    }
    case "badge_label":
      return { field: "label", why: t("staff_roles.badge.label.rule", { max: roleLimits.badgeLabel }) };
    case "badge_color":
      return { field: "color", why: t("staff_roles.badge.color.refused") };
    case "builtin":
      return { why: t("staff_roles.save.builtin") };
    default:
  }
  if (roleManagedByPlan(error)) return { why: t("staff_roles.save.plan") };
  if (error instanceof ProblemError && error.status === 403) return { why: t("staff_roles.save.forbidden") };
  return { why: t("staff_roles.save.failed") };
}

/** The state of a role's form, its focus on a refused field, and whether it holds changes not saved. */
function useRoleForm(start: RoleForm, onUnsaved: AdminSubProps["onUnsaved"]) {
  const { t, locale } = useStaffStrings();
  const id = useId();
  const [base, setBase] = useState(start);
  const [form, setForm] = useState(start);
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  const focusNext = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (focusNext.current === undefined) return;
    focusRefusedField(focusNext.current);
    focusNext.current = undefined;
  });
  const busy = sent.kind === "sending";
  const unsaved =
    form.key !== base.key ||
    form.position !== base.position ||
    !samePermissions(form.permissions, base.permissions) ||
    form.label.trim() !== base.label.trim() ||
    (form.label.trim() !== "" && form.color !== base.color);
  useUnsavedReport(onUnsaved, unsaved, busy);
  /** The id of the control a refusal of `field` puts the focus on: of the permissions, the first offered. */
  const controlOf = (field: Field, rights: RoleRights) =>
    field === "permissions"
      ? `${id}-permission-${offeredPermissionsAt(rights, form.key, form.position)[0] ?? permissionOrder[0]}`
      : field === "position" || field === "color"
        ? `${id}-${field}-control`
        : `${id}-${field}`;
  const refuse = (field: Field | undefined, why: string, rights: RoleRights) => {
    setSent({ kind: "refused", field, why });
    if (field !== undefined) focusNext.current = controlOf(field, rights);
  };
  return {
    id,
    base,
    form,
    sent,
    busy,
    unsaved,
    setBase,
    setSent,
    change: (part: Partial<RoleForm>) => {
      setForm((now) => ({ ...now, ...part }));
      setSent((was) => (was.kind === "saved" || was.kind === "refused" ? { kind: "idle" } : was));
    },
    /** Says what came of a refused write: under the field it is about, which takes the focus, or in the submit bar. */
    refused: (error: unknown, rights: RoleRights) => {
      const r = refusalOf(t, locale, error, form);
      refuse(r.field, r.why, rights);
    },
    /** What the form finds wrong before sending: a key the server would refuse, a place out of reach or none. */
    check: (rights: RoleRights, keyed: boolean): boolean => {
      if (keyed && !validRoleKey(form.key)) {
        refuse("key", t("staff_roles.key.rule"), rights);
        return false;
      }
      if (form.position === null || form.position < 0 || form.position > highestPlace(rights)) {
        refuse("position", t("staff_roles.position.reach", { max: highestPlace(rights) }), rights);
        return false;
      }
      if (isStaffRole(form.key, form.permissions) && form.position < lowestStaffPlace(rights)) {
        refuse("position", t("staff_roles.position.plans", { min: lowestStaffPlace(rights) }), rights);
        return false;
      }
      if (Array.from(form.label.trim()).length > roleLimits.badgeLabel || form.label.includes("\n")) {
        refuse("label", t("staff_roles.badge.label.rule", { max: roleLimits.badgeLabel }), rights);
        return false;
      }
      return true;
    },
  };
}

type FormState = ReturnType<typeof useRoleForm>;

/** What is wrong with `field`, as the form's last save says it. */
const errorOf = (f: FormState, field: Field): string | undefined =>
  f.sent.kind === "refused" && f.sent.field === field ? f.sent.why : undefined;

/** The lowest place the form's role may take: above the plans' roles for a staff role, else 0. */
const lowestPlaceOf = (f: FormState, rights: RoleRights): number =>
  isStaffRole(f.form.key, f.form.permissions) ? Math.min(lowestStaffPlace(rights), highestPlace(rights)) : 0;

/** The place of a role among the roles, no higher than the reader's highest. */
function PlaceRow({ f, rights }: { f: FormState; rights: RoleRights }) {
  const { t } = useStaffStrings();
  return (
    <SettingRow
      id={`${f.id}-position`}
      label={t("staff_roles.position")}
      about={t("staff_roles.position.about", { min: lowestPlaceOf(f, rights), max: highestPlace(rights) })}
      form
      error={errorOf(f, "position")}
    >
      <NumberField
        id={`${f.id}-position-control`}
        name="position"
        value={f.form.position}
        min={lowestPlaceOf(f, rights)}
        max={highestPlace(rights)}
        isDisabled={f.busy}
        onChange={(position) => {
          f.change({ position });
        }}
      />
    </SettingRow>
  );
}

/**
 * The permissions of a role: a checkbox for each the reader holds, which they may give or take; and those the role
 * holds besides, which the reader lacks, named in a sentence and kept as they are.
 */
function PermissionFields({ f, rights, fixed }: { f: FormState; rights: RoleRights; fixed: readonly Permission[] }) {
  const { t, locale } = useStaffStrings();
  const error = errorOf(f, "permissions");
  const offered = offeredPermissionsAt(rights, f.form.key, f.form.position);
  return (
    <SettingsSection heading={t("staff_roles.permissions")} about={t("staff_roles.permissions.about")}>
      {offered.map((p, i) => (
        <CheckField
          key={p}
          id={`${f.id}-permission-${p}`}
          name="permissions"
          value={p}
          label={permissionName(t, p)}
          checked={f.form.permissions.has(p)}
          touch
          disabled={f.busy}
          // The refusal of the permissions as a whole stands under the first of them.
          error={i === 0 ? error : undefined}
          onChange={(on) => {
            const next = new Set(f.form.permissions);
            if (on) next.add(p);
            else next.delete(p);
            f.change({ permissions: next });
          }}
        />
      ))}
      {fixed.length > 0 && (
        <SettingsNote>{t("staff_roles.permissions.fixed", { names: permissionList(t, locale, fixed) })}</SettingsNote>
      )}
    </SettingsSection>
  );
}

/** The badge of a role: its label, empty for none, its color, and how it shows. */
function BadgeFields({ f }: { f: FormState }) {
  const { t } = useStaffStrings();
  const label = f.form.label.trim();
  return (
    <SettingsSection heading={t("staff_roles.badge")} about={t("staff_roles.badge.about")}>
      <TextField
        id={`${f.id}-label`}
        name="label"
        label={t("staff_roles.badge.label")}
        row
        defaultValue={f.form.label}
        maxLength={roleLimits.badgeLabel}
        error={errorOf(f, "label")}
        readOnly={f.busy}
        onChange={(value) => {
          f.change({ label: value });
        }}
      />
      {label !== "" && (
        <SettingRow
          id={`${f.id}-color`}
          label={t("staff_roles.badge.color")}
          form
          error={errorOf(f, "color")}
          values={<Badge>{label}</Badge>}
        >
          <Select
            id={`${f.id}-color-control`}
            name="color"
            options={badgeColors.map((color) => ({
              value: color,
              label: t("staff_roles.badge.color.name", { color }),
            }))}
            value={f.form.color}
            isDisabled={f.busy}
            onChange={(v) => {
              const color = badgeColors.find((c) => c === v);
              if (color !== undefined) f.change({ color });
            }}
          />
        </SettingRow>
      )}
    </SettingsSection>
  );
}

/** The submit bar of a role's form. */
function RoleBar({ f, onSave, label }: { f: FormState; onSave: () => void; label: string }) {
  const { t } = useStaffStrings();
  return (
    <SubmitBar
      result={
        f.sent.kind === "saved" ? (
          <SaveState state="saved" />
        ) : f.sent.kind === "refused" ? (
          <FormAlert>{f.sent.field === undefined ? f.sent.why : t("staff_roles.save.refused")}</FormAlert>
        ) : undefined
      }
    >
      <Button kind="primary" busy={f.busy} onPress={onSave}>
        {label}
      </Button>
    </SubmitBar>
  );
}

/** The way back to the roles. */
function BackToRoles({ go }: { go: (href: string) => void }) {
  return <BackTo go={go} href={adminRolesPath} name="staff_roles.entry" />;
}

/** A new role: its key, its place and its permissions. */
export function NewRoleScreen({ go, onUnsaved, leaving }: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminRoles();
  const writes = useAdminRoleWrites();
  const there = useStillThere();
  const f = useRoleForm({ key: "", position: 0, permissions: new Set(), label: "", color: "neutral" }, onUnsaved);
  const title = t("staff_roles.new");
  const rights = read.rights;
  if (rights === undefined)
    return (
      <SettingsScreen
        sub
        title={title}
        back={<BackToRoles go={go} />}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("staff_roles.loading"), failed: t("staff_roles.failed") }}
          />
        }
      />
    );
  const save = () => {
    if (f.busy || !f.check(rights, true)) return;
    f.setSent({ kind: "sending" });
    const permissions = permissionOrder.filter((p) => f.form.permissions.has(p));
    writes.create({ key: f.form.key, position: f.form.position ?? 0, permissions }).then(
      (role) => {
        if (!there.current) return;
        f.setBase(f.form);
        f.setSent({ kind: "saved" });
        // The guard hears that nothing is left unsaved before the screen leaves, not at the next render.
        onUnsaved?.(false);
        go(adminRolePath(role.key));
      },
      (error: unknown) => {
        if (there.current) f.refused(error, rights);
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={title}
        back={<BackToRoles go={go} />}
        foot={<RoleBar f={f} onSave={save} label={t("staff_roles.create")} />}
      >
        <SettingsSection heading={t("staff_roles.role")}>
          <TextField
            id={`${f.id}-key`}
            name="key"
            label={t("staff_roles.key")}
            hint={t("staff_roles.key.rule")}
            row
            defaultValue=""
            maxLength={roleLimits.key}
            autoCapitalize="none"
            error={errorOf(f, "key")}
            readOnly={f.busy}
            onChange={(key) => {
              f.change({ key });
            }}
          />
          <PlaceRow f={f} rights={rights} />
        </SettingsSection>
        <PermissionFields f={f} rights={rights} fixed={[]} />
      </SettingsScreen>
      <ModalPresence>{leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />}</ModalPresence>
    </>
  );
}

/** What a change of the form sends: only what changed, the badge removed where its label was emptied. */
function changeOf(f: FormState, reshape: boolean): RoleChange {
  const { form, base } = f;
  const label = form.label.trim();
  const badgeChanged = label !== base.label.trim() || (label !== "" && form.color !== base.color);
  return {
    ...(reshape && form.position !== base.position && form.position !== null ? { position: form.position } : {}),
    ...(reshape && !samePermissions(form.permissions, base.permissions)
      ? { permissions: permissionOrder.filter((p) => form.permissions.has(p)) }
      : {}),
    ...(!badgeChanged ? {} : label === "" ? { remove_badge: true } : { badge: { label, color: form.color } }),
  };
}

/** What a role that cannot be changed is: its place, its permissions and its badge, and why the reader cannot. */
function RoleFacts({ role, why }: { role: AdminRole; why: string }) {
  const { t, locale } = useStaffStrings();
  return (
    <SettingsSection heading={t("staff_roles.role")}>
      <SettingsFacts
        label={t("staff_roles.role")}
        items={[
          { text: t("staff_roles.row.place", { position: role.position }) },
          {
            text:
              role.permissions.length === 0
                ? t("staff_roles.facts.no_permissions")
                : t("staff_roles.facts.permissions", { names: permissionList(t, locale, role.permissions) }),
          },
        ]}
      />
      <SettingsNote>{why}</SettingsNote>
    </SettingsSection>
  );
}

/** A role the reader may change: its form, and Delete last where the reader may delete it. */
function RoleForm({ role, rights, go, onUnsaved, leaving }: AdminSubProps & { role: AdminRole; rights: RoleRights }) {
  const { t } = useStaffStrings();
  const writes = useAdminRoleWrites();
  const there = useStillThere();
  const f = useRoleForm(
    {
      key: role.key,
      position: role.position,
      permissions: new Set(role.permissions),
      label: role.badge?.label ?? "",
      color: role.badge?.color ?? "neutral",
    },
    onUnsaved,
  );
  const reshape = mayReshape(role, rights);
  const why = fixedReason(t, role, rights);
  const [asking, setAsking] = useState(false);
  const [deleting, setDeleting] = useState<"idle" | "sending" | { failed: string }>("idle");
  const save = () => {
    if (f.busy || (reshape && !f.check(rights, false))) return;
    const change = changeOf(f, reshape);
    if (Object.keys(change).length === 0) {
      go(adminRolesPath);
      return;
    }
    f.setSent({ kind: "sending" });
    writes.change(role.key, change).then(
      () => {
        if (!there.current) return;
        f.setBase(f.form);
        f.setSent({ kind: "saved" });
      },
      (error: unknown) => {
        if (there.current) f.refused(error, rights);
      },
    );
  };
  const remove = () => {
    setDeleting("sending");
    writes.remove(role.key).then(
      () => {
        if (!there.current) return;
        setAsking(false);
        onUnsaved?.(false);
        go(adminRolesPath);
      },
      (error: unknown) => {
        if (!there.current) return;
        setAsking(false);
        setDeleting({
          failed: roleRefusal(error) === "held" ? t("staff_roles.delete.held") : t("staff_roles.delete.failed"),
        });
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={role.key}
        back={<BackToRoles go={go} />}
        // What ends the role stands last and apart, after the form and its Save.
        foot={
          <>
            <RoleBar f={f} onSave={save} label={t("staff_roles.save")} />
            {mayDelete(role, rights) && (
              <SettingsSection heading={t("staff_roles.delete")} about={t("staff_roles.delete.about")}>
                <Irreversible>{t("staff_roles.irreversible")}</Irreversible>
                <Box>
                  <Button
                    size="timeline"
                    touch
                    isDisabled={f.busy}
                    onPress={() => {
                      setAsking(true);
                    }}
                  >
                    {t("staff_roles.delete.press")}
                  </Button>
                </Box>
                <SettingsResult>{typeof deleting === "object" ? deleting.failed : undefined}</SettingsResult>
              </SettingsSection>
            )}
          </>
        }
      >
        {reshape ? (
          <>
            <SettingsSection heading={t("staff_roles.role")}>
              <PlaceRow f={f} rights={rights} />
            </SettingsSection>
            <PermissionFields f={f} rights={rights} fixed={fixedPermissions(role, rights)} />
          </>
        ) : (
          <RoleFacts role={role} why={why ?? ""} />
        )}
        <BadgeFields f={f} />
      </SettingsScreen>
      <ModalPresence>
        {asking ? (
          <ConfirmDialog
            key="delete"
            title={t("staff_roles.delete.title", { key: role.key })}
            cancel={t("staff_roles.delete.cancel")}
            confirm={t("staff_roles.delete.press")}
            phrase={role.key}
            busy={deleting === "sending"}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={remove}
          >
            {t("staff_roles.delete.text")}
          </ConfirmDialog>
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/** The page of the role of `key`: its form, what it is where the reader may not change it, or that it loads or is none. */
export function RoleScreen({ roleKey, ...sub }: AdminSubProps & { roleKey: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminRole(roleKey);
  const { role, rights } = read;
  if (role === undefined || rights === undefined)
    return (
      <SettingsScreen
        sub
        title={read.missing ? t("staff_roles.entry") : roleKey}
        back={<BackToRoles go={sub.go} />}
        notice={
          <ScreenNotice
            status={read.missing ? "missing" : read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_roles.loading"),
              failed: t("staff_roles.failed"),
              missing: t("staff_roles.missing"),
            }}
            all={{ label: t("staff_roles.all"), href: adminRolesPath }}
          />
        }
      />
    );
  if (!withinReach(role, rights))
    return (
      <SettingsScreen sub title={role.key} back={<BackToRoles go={sub.go} />}>
        <RoleFacts role={role} why={fixedReason(t, role, rights) ?? ""} />
      </SettingsScreen>
    );
  return <RoleForm key={role.id} role={role} rights={rights} {...sub} />;
}
