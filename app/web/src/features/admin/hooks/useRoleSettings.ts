// SPDX-License-Identifier: AGPL-3.0-only
// The roles screen (FR-720, manage_roles): the roles of the instance, what the viewer may do with each, creating a
// role, changing its position and permissions, giving it a badge (ADR-0071), and deleting it. The viewer changes only
// the roles at or below their highest one and only the permissions their roles grant; admin changes only its badge,
// and neither built-in role is deleted. A refused input stays next to its field; what a change did is announced and kept in a StatusMessage.
import { type BadgeColor, useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
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

/** The fields of a role that the form checks or the server may refuse, and the rest of the form. */
type RoleField = "key" | "position" | "permissions" | "badge" | "form";

/** Why a role was refused: the field, the localized reason, and the permissions it names. */
export type RoleFieldError = { field: RoleField; message: string; missing: string[] };

/** A role as the form edits it; the position stays the text typed until it is sent. */
export type RoleDraft = { key: string; position: string; permissions: readonly Permission[] };

const emptyDraft: RoleDraft = { key: "", position: "", permissions: [] };

/** The draft of an existing role. */
export function draftOfRole(role: AdminRole): RoleDraft {
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
    return { field: "key", message: t("client:admin.roles.key.invalid", { max: String(roleKeyLimit) }), missing: [] };
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

/** The form of a new role. A success empties it and says so in the form's StatusMessage. */
export function useNewRoleForm(rights: RoleRights) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [draft, setDraft] = useState<RoleDraft>(emptyDraft);
  const [invalid, setInvalid] = useState<RoleFieldError | undefined>(undefined);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...createRoleMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (role) => {
      await refreshRoles(api, queryClient);
      setDraft(emptyDraft);
      const message = t("client:admin.roles.created", { role: isolatedRoleKey(role.key) });
      setStatus(message);
      notice.inPlace(message);
    },
  });
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
    error: invalid ?? (mutation.isError ? refusal(mutation.error, t) : undefined),
    /** What the last submit did, for the form's StatusMessage. */
    status,
  };
}

/** A badge as the form edits it. */
export type BadgeDraft = { label: string; color: BadgeColor };

/** The draft of a role's badge: its badge, or a neutral one without a name. */
export function draftOfBadge(role: AdminRole): BadgeDraft {
  return role.badge === undefined
    ? { label: "", color: "neutral" }
    : { label: role.badge.label, color: role.badge.color };
}

/** Checks the name of a badge before it is sent: 1 to 32 characters (code points, as the server counts) once trimmed. */
function checkBadge(draft: BadgeDraft, t: Translate): RoleFieldError | undefined {
  const length = Array.from(draft.label.trim()).length;
  if (length === 0 || length > badgeLabelLimit) {
    return {
      field: "badge",
      message: t("client:admin.roles.badge.label.invalid", { max: String(badgeLabelLimit) }),
      missing: [],
    };
  }
  return undefined;
}

export type RoleActions = {
  /** Checks and sends a change of a role; resolves with the refused field, or undefined once it is saved. */
  update: (role: AdminRole, draft: RoleDraft) => Promise<RoleFieldError | undefined>;
  /** Sets the badge of a role, or removes it for null; resolves with the refusal, or undefined once it is saved. */
  updateBadge: (role: AdminRole, draft: BadgeDraft | null) => Promise<RoleFieldError | undefined>;
  /** Deletes a role; resolves with why it failed, or undefined once it is deleted. */
  remove: (role: AdminRole) => Promise<string | undefined>;
  /** What the last change did, for the list's StatusMessage. */
  status: string | undefined;
};

/**
 * Changing and deleting the roles of the list. `onDeleted(id)` is called once a deleted role is about to leave the
 * list (to keep focus, lib/useFocusAfterRemoval); a changed role keeps its row, which shows the answer.
 */
export function useRoleActions(rights: RoleRights, onDeleted: (id: string) => void): RoleActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const update = useMutation(updateRoleMutation(api));
  const remove = useMutation(deleteRoleMutation(api));
  const report = (message: string) => {
    setStatus(message);
    notice.inPlace(message);
  };
  return {
    update: async (role, draft) => {
      const checked = checkDraft(draft, rights, t, false);
      if (checked !== undefined) return checked;
      const position = Number(draft.position.trim());
      const change: RoleChange = {
        permissions: [...draft.permissions],
        ...(position === role.position ? {} : { position }),
      };
      try {
        const saved = await update.mutateAsync({ key: role.key, change });
        await refreshRoles(api, queryClient);
        report(t("client:admin.roles.updated", { role: isolatedRoleKey(saved.key) }));
        return undefined;
      } catch (error) {
        return refusal(error, t);
      }
    },
    updateBadge: async (role, draft) => {
      const checked = draft === null ? undefined : checkBadge(draft, t);
      if (checked !== undefined) return checked;
      const change: RoleChange =
        draft === null ? { remove_badge: true } : { badge: { label: draft.label.trim(), color: draft.color } };
      try {
        const saved = await update.mutateAsync({ key: role.key, change });
        await refreshRoles(api, queryClient);
        report(t("client:admin.roles.updated", { role: isolatedRoleKey(saved.key) }));
        return undefined;
      } catch (error) {
        return refusal(error, t);
      }
    },
    remove: async (role) => {
      try {
        await remove.mutateAsync(role.key);
      } catch (error) {
        return refusal(error, t).message;
      }
      onDeleted(role.id);
      await refreshRoles(api, queryClient);
      report(t("client:admin.roles.deleted", { role: isolatedRoleKey(role.key) }));
      return undefined;
    },
    status,
  };
}
