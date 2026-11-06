// SPDX-License-Identifier: AGPL-3.0-only
// The roles screen (FR-720, manage_roles): the roles of the instance, what the viewer may do with each, creating a
// role (its own page), and, on a role's page, changing its position, permissions and badge (ADR-0070) in one form and
// deleting it. The viewer changes only the roles at or below their highest one and only the permissions their roles
// grant; admin changes only its badge, and neither built-in role is deleted. A refused input stays next to its field;
// the result of a save shows beside its button (FormActionBar); creating and deleting move to another page with a toast.
import { type BadgeColor, type FormActionResult, useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { accountRolesQueryOptions, type Permission, rolesQueryOptions } from "../../moderation";
import { viewerQueryOptions } from "../../session";
import {
  adminRoleKey,
  type AdminRole,
  allPermissions,
  badgeLabelLimit,
  createRoleMutation,
  deleteRoleMutation,
  maxRolePosition,
  moderatorRoleKey,
  refreshRoles,
  type RoleChange,
  roleKeyLimit,
  reservedRoleKey,
  roleKeyPattern,
  roleProblem,
  updateRoleMutation,
} from "../api/roles";

/** A role's key inside a sentence, isolated left to right (U+2066 … U+2069). */
export function isolatedRoleKey(key: string): string {
  return `⁦${key}⁩`;
}

/** What the viewer may do with the roles: their highest role, the permissions their roles grant, the highest position they may give. */
export type RoleRights = {
  top: AdminRole | undefined;
  permissions: readonly Permission[];
  maxPosition: number;
};

/** One role as the screen shows it, with what the viewer may do with it. */
export type RoleRow = {
  role: AdminRole;
  /** Whether the viewer may change its position and permissions (never admin). */
  canChange: boolean;
  /** Whether the viewer may change its badge (admin included). */
  canChangeBadge: boolean;
  /** Whether the viewer may delete it (never a built-in role). */
  canDelete: boolean;
  /** The viewer's highest role when this one is above it, else undefined. */
  outrankedBy: AdminRole | undefined;
};

/** The roles of the instance, highest first, and the viewer's rights; the route's loader has loaded both. */
export function useRoleSettings(): { rows: RoleRow[]; rights: RoleRights } {
  const api = useApi();
  const viewer = useSuspenseQuery(viewerQueryOptions(api)).data;
  const roles = useSuspenseQuery(rolesQueryOptions(api)).data.items;
  const own = useSuspenseQuery(accountRolesQueryOptions(api, viewer.id)).data.items;
  const top = own.reduce<AdminRole | undefined>(
    (high, role) => (high === undefined || role.position > high.position ? role : high),
    undefined,
  );
  const rights: RoleRights = {
    top,
    permissions: viewer.permissions,
    maxPosition: Math.min(maxRolePosition, top?.position ?? -1),
  };
  return {
    rights,
    rows: roles.map((role) => {
      const outrankedBy = top === undefined || role.position > top.position ? top : undefined;
      const builtin = role.key === adminRoleKey || role.key === moderatorRoleKey;
      const allowed = top !== undefined && outrankedBy === undefined;
      return {
        role,
        outrankedBy,
        canChange: allowed && role.key !== adminRoleKey,
        canChangeBadge: allowed,
        canDelete: allowed && !builtin,
      };
    }),
  };
}

/** One role of the instance by its key, with the viewer's rights; undefined for a key no role has. */
export function useRole(key: string): { row: RoleRow | undefined; rights: RoleRights } {
  const { rows, rights } = useRoleSettings();
  return { row: rows.find((row) => row.role.key === key), rights };
}

/** The address of a role's page. */
export function rolePath(key: string): string {
  return `/admin/roles/${encodeURIComponent(key)}`;
}

/** The fields of a role that the form checks or the server may refuse, and the rest of the form. */
type RoleField = "key" | "position" | "permissions" | "badge" | "form";

/** Why a role was refused: the field, the localized reason, and the permissions it names. */
export type RoleFieldError = { field: RoleField; message: string; missing: string[] };

/** A role as the form edits it; the position stays the text typed until it is sent. */
export type RoleDraft = { key: string; position: string; permissions: readonly Permission[] };

const emptyDraft: RoleDraft = { key: "", position: "", permissions: [] };

/** The draft of an existing role. */
function draftOfRole(role: AdminRole): RoleDraft {
  return { key: role.key, position: String(role.position), permissions: role.permissions };
}

/** The permissions of a draft with `permission` chosen or not, in the order of the API's `Permission`. */
export function togglePermission(
  permissions: readonly Permission[],
  permission: Permission,
  selected: boolean,
): Permission[] {
  return allPermissions.filter((p) => (p === permission ? selected : permissions.includes(p)));
}

type Translate = ReturnType<typeof useT>;

/** Which field a refusal of the server is about, with its reason. */
function refusal(error: unknown, t: Translate): RoleFieldError {
  if (error instanceof ApiError && error.status === 403 && error.type === "about:blank") {
    return { field: "form", message: t("client:admin.roles.failed.outranked"), missing: [] };
  }
  const problem = roleProblem(error);
  const message = errorMessage(error);
  switch (problem?.reason) {
    case "key":
    case "key_taken":
      return { field: "key", message, missing: [] };
    case "position":
      return { field: "position", message, missing: [] };
    case "permissions":
    case "missing_permission":
      return { field: "permissions", message, missing: problem.missing };
    case "badge_label":
    case "badge_color":
      return { field: "badge", message, missing: [] };
    default:
      return { field: "form", message, missing: [] };
  }
}

/** The position typed in a draft, or undefined when it is not a whole number from 0 to `max`. */
function parsePosition(text: string, max: number): number | undefined {
  const trimmed = text.trim();
  if (!/^\d+$/.test(trimmed)) return undefined;
  const position = Number(trimmed);
  return position <= max ? position : undefined;
}

/** Checks a draft before it is sent: the key of a new role, and the position. */
function checkDraft(draft: RoleDraft, rights: RoleRights, t: Translate, isNew: boolean): RoleFieldError | undefined {
  const key = draft.key.trim();
  if (isNew && (key === "" || key.length > roleKeyLimit || !roleKeyPattern.test(key))) {
    return {
      field: "key",
      message: t("client:admin.roles.key.invalid", { max: String(roleKeyLimit) }),
      missing: [],
    };
  }
  if (isNew && key === reservedRoleKey) {
    return { field: "key", message: t("client:admin.roles.key.reserved"), missing: [] };
  }
  if (parsePosition(draft.position, rights.maxPosition) === undefined) {
    return {
      field: "position",
      message: t("client:admin.roles.position.invalid", { max: String(rights.maxPosition) }),
      missing: [],
    };
  }
  return undefined;
}

/** The result of a refusal of the whole form (not of one field), for the form's FormActionBar. */
function formResult(error: RoleFieldError | undefined, t: Translate): FormActionResult | undefined {
  return error?.field === "form"
    ? { tone: "danger", message: t("client:admin.roles.save_failed", { reason: error.message }) }
    : undefined;
}

/**
 * The form of a new role. Once it is created, the role's page opens (web-client §16 G24, the result is the page it
 * moves to) and a toast says so; a refusal stays next to its field, or beside the button, with the draft.
 */
export function useNewRoleForm(rights: RoleRights) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const notice = useNotice();
  const [draft, setDraft] = useState<RoleDraft>(emptyDraft);
  const [invalid, setInvalid] = useState<RoleFieldError | undefined>(undefined);
  const mutation = useMutation({
    ...createRoleMutation(api),
    onSuccess: async (role) => {
      await refreshRoles(api, queryClient);
      await navigate({ to: "/admin/roles/$key", params: { key: role.key } });
      notice.toast({
        tone: "success",
        message: t("client:admin.roles.created", { role: isolatedRoleKey(role.key) }),
      });
    },
  });
  const error = invalid ?? (mutation.isError ? refusal(mutation.error, t) : undefined);
  return {
    draft,
    change: (change: Partial<RoleDraft>) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (mutation.isPending) return;
      const checked = checkDraft(draft, rights, t, true);
      setInvalid(checked);
      if (checked !== undefined) {
        mutation.reset();
        return;
      }
      mutation.mutate({
        key: draft.key.trim(),
        position: Number(draft.position.trim()),
        permissions: [...draft.permissions],
      });
    },
    isPending: mutation.isPending,
    /** Why the role was refused, next to its field; the draft stays. */
    error,
    /** A refusal of the whole form, beside the button. */
    result: formResult(error, t),
  };
}

/** A badge as the form edits it. */
export type BadgeDraft = { label: string; color: BadgeColor };

/** The draft of a role's badge: its badge, or a neutral one without a name. */
function draftOfBadge(role: AdminRole): BadgeDraft {
  return role.badge === undefined
    ? { label: "", color: "neutral" }
    : { label: role.badge.label, color: role.badge.color };
}

/** Checks the name of a badge before it is sent: at most 32 characters (code points, as the server counts) once trimmed. */
function checkBadge(draft: BadgeDraft, t: Translate): RoleFieldError | undefined {
  const length = Array.from(draft.label.trim()).length;
  if (length > badgeLabelLimit) {
    return {
      field: "badge",
      message: t("client:admin.roles.badge.label.invalid", { max: String(badgeLabelLimit) }),
      missing: [],
    };
  }
  return undefined;
}

/** A role as its page's form edits it: its position and permissions, and its badge (no name for none). */
export type RoleFormDraft = RoleDraft & { badge: BadgeDraft };

/**
 * The form of a role's page: its position and permissions (never for admin) and its badge, saved with one button and
 * one PATCH; the result shows beside the button (FormActionBar), a refusal next to its field. An empty name removes
 * the badge.
 */
export function useRoleForm(row: RoleRow, rights: RoleRights) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const { role } = row;
  const [draft, setDraft] = useState<RoleFormDraft>(() => ({
    ...draftOfRole(role),
    badge: draftOfBadge(role),
  }));
  const [error, setError] = useState<RoleFieldError | undefined>(undefined);
  const [result, setResult] = useState<FormActionResult | undefined>(undefined);
  const update = useMutation(updateRoleMutation(api));
  /** The change to send, or the refused field. */
  const changeOf = (): RoleChange | RoleFieldError => {
    const checked = (row.canChange ? checkDraft(draft, rights, t, false) : undefined) ?? checkBadge(draft.badge, t);
    if (checked !== undefined) return checked;
    const label = draft.badge.label.trim();
    const position = Number(draft.position.trim());
    return {
      ...(row.canChange && {
        permissions: [...draft.permissions],
        ...(position === role.position ? {} : { position }),
      }),
      ...(label === ""
        ? role.badge === undefined
          ? {}
          : { remove_badge: true }
        : { badge: { label, color: draft.badge.color } }),
    };
  };
  return {
    draft,
    change: (change: Partial<RoleDraft>) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    changeBadge: (change: Partial<BadgeDraft>) => {
      setDraft((previous) => ({ ...previous, badge: { ...previous.badge, ...change } }));
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (update.isPending) return;
      setResult(undefined);
      const change = changeOf();
      if ("field" in change) {
        setError(change);
        return;
      }
      setError(undefined);
      void update.mutateAsync({ key: role.key, change }).then(
        async (saved) => {
          await refreshRoles(api, queryClient);
          setResult({
            tone: "success",
            message: t("client:admin.roles.updated", { role: isolatedRoleKey(saved.key) }),
          });
        },
        (failure: unknown) => {
          const refused = refusal(failure, t);
          setError(refused);
          setResult(formResult(refused, t));
        },
      );
    },
    isPending: update.isPending,
    /** Why the role was refused, next to its field; the draft stays. */
    error,
    /** The result of the last save, beside the button. */
    result,
  };
}

/**
 * Deleting a role from its page. Resolves with why it failed (the alert dialog stays open with it); once it is
 * deleted, the list of roles opens and a toast says so.
 */
export function useDeleteRole(role: AdminRole) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const notice = useNotice();
  const remove = useMutation(deleteRoleMutation(api));
  return {
    remove: async (): Promise<string | undefined> => {
      try {
        await remove.mutateAsync(role.key);
      } catch (error) {
        return refusal(error, t).message;
      }
      // The page leaves before the roles are fetched again, so that it never shows a role that is gone.
      await navigate({ to: "/admin/roles" });
      notice.toast({
        tone: "success",
        message: t("client:admin.roles.deleted", { role: isolatedRoleKey(role.key) }),
      });
      await refreshRoles(api, queryClient);
      return undefined;
    },
    isPending: remove.isPending,
  };
}
