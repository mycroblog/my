// SPDX-License-Identifier: AGPL-3.0-only
// The roles of a local account on its admin page (FR-720, manage_roles): every role of the instance, whether the
// account's user holds it, and whether the viewer may change it (only the roles at or below the viewer's highest
// one). Granting and revoking ask first in a confirmation; a refusal stays in it with the reason (E2), and what a
// change did stays in the region's StatusMessage.
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { viewerQueryOptions } from "../../session";
import {
  accountRolesQueryOptions,
  type AdminRole,
  type AdminRoleList,
  cacheChangedRoles,
  changeRoleMutation,
  isLastAdmin,
  isRoleOutranked,
  refreshViewerRoles,
  type RoleChangeKind,
  rolesQueryOptions,
} from "../api/roles";

/** One role of the instance as the account's page shows it. */
export type RoleRow = {
  role: AdminRole;
  /** Whether the account's user holds it. */
  held: boolean;
  /** The viewer's highest role when it is below this one (the viewer may not change it), else undefined. */
  outrankedBy: AdminRole | undefined;
  /** The permissions of the role that the viewer lacks (the server grants a role only to a viewer who holds all of them). */
  missing: string[];
};

/** The roles of an account: loading, failing to load, or every role of the instance. */
export type AccountRoles =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | { status: "ready"; rows: RoleRow[]; held: AdminRole[] };

/** The highest of `roles` (the API lists them highest first), or undefined for none. */
function highest(roles: readonly AdminRole[]): AdminRole | undefined {
  return roles.reduce<AdminRole | undefined>((top, role) => (top === undefined || role.position > top.position ? role : top), undefined);
}

/**
 * The roles of the instance with those that local account `accountId` holds, and the viewer's highest role,
 * which decides the roles the viewer may change. The page's loader has loaded the viewer's account.
 */
export function useAccountRoles(accountId: string): AccountRoles {
  const api = useApi();
  const viewer = useSuspenseQuery(viewerQueryOptions(api)).data;
  const all = useQuery(rolesQueryOptions(api));
  const held = useQuery(accountRolesQueryOptions(api, accountId));
  const own = useQuery(accountRolesQueryOptions(api, viewer.id));
  const queries = [all, held, own];
  if (all.data === undefined || held.data === undefined || own.data === undefined) {
    const failed = queries.find((query) => query.isError);
    if (failed === undefined) return { status: "loading" };
    return {
      status: "error",
      reason: errorMessage(failed.error),
      retry: () => {
        for (const query of queries) if (query.isError) void query.refetch();
      },
      isRetrying: queries.some((query) => query.isFetching),
    };
  }
  const heldIds = new Set(held.data.items.map((role) => role.id));
  const top = highest(own.data.items);
  const permissions = new Set(own.data.items.flatMap((role) => role.permissions));
  return {
    status: "ready",
    held: held.data.items,
    rows: all.data.items.map((role) => ({
      role,
      held: heldIds.has(role.id),
      outrankedBy: top === undefined || role.position > top.position ? top : undefined,
      missing: role.permissions.filter((permission) => !permissions.has(permission)),
    })),
  };
}

/** The change being confirmed: of which role, and whether it is granted or revoked. */
type Confirmation = { role: AdminRole; kind: RoleChangeKind };

/** The confirmation dialog: open, or closing and still showing the last confirmation while it fades out. */
type DialogState = { isOpen: boolean; confirmation: Confirmation } | null;

export type RoleChanges = {
  /** Opens the confirmation of granting or revoking `role`. */
  open: (role: AdminRole, kind: RoleChangeKind) => void;
  isOpen: boolean;
  /** The confirmation open now or last, which a closing dialog still shows; null before the first one. */
  confirmation: Confirmation | null;
  /** Sends the change; on success the confirmation closes and the region shows the roles after it. */
  confirm: () => void;
  cancel: () => void;
  isPending: boolean;
  /** Why the change failed, localized, for the confirmation (which stays open). */
  error: string | undefined;
  /** What the last change did, for the region's StatusMessage; undefined before the first one. */
  status: string | undefined;
};

/**
 * Granting and revoking the roles of local account `accountId`, whose handle (isolated) is `handle`. `onChanged()`
 * is called when the roles after a change are in the cache and the pressed button gives way to the other one.
 * The account's page (its role keys) and the audit log, which records the change, are read again first; a change
 * of the viewer's own roles also reads the viewer's permissions again.
 */
export function useRoleChanges(accountId: string, handle: string, onChanged: () => void): RoleChanges {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const viewer = useSuspenseQuery(viewerQueryOptions(api)).data;
  const [dialog, setDialog] = useState<DialogState>(null);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...changeRoleMutation(api),
    onSuccess: async (roles: AdminRoleList, change) => {
      // The change is done once what it changes is in the cache (the confirmation stays pending until then), so
      // that the audit log shows it at once (cacheChangedRoles).
      await cacheChangedRoles(api, queryClient, accountId, roles, viewer.id);
      setDialog((current) => current && { ...current, isOpen: false });
      const role = `⁦${change.role}⁩`;
      setStatus(
        change.kind === "grant" ? t("client:admin.account.roles.granted", { role, handle }) : t("client:admin.account.roles.revoked", { role, handle }),
      );
      onChanged();
    },
    onError: (error) => {
      // The viewer's roles changed since they loaded: read them again, so that the region shows what is allowed now.
      if (isRoleOutranked(error)) refreshViewerRoles(api, queryClient, viewer.id);
    },
  });
  const isOpen = dialog?.isOpen ?? false;
  const failure = (error: unknown): string => {
    if (isRoleOutranked(error)) return t("client:admin.account.roles.failed.outranked");
    if (isLastAdmin(error)) return t("client:admin.account.roles.failed.last_admin");
    return t("client:admin.account.roles.failed", { reason: errorMessage(error) });
  };
  return {
    open: (role, kind) => {
      mutation.reset();
      setDialog({ isOpen: true, confirmation: { role, kind } });
    },
    isOpen,
    confirmation: dialog?.confirmation ?? null,
    confirm: () => {
      if (dialog === null || !dialog.isOpen || mutation.isPending) return;
      mutation.mutate({ accountId, role: dialog.confirmation.role.key, kind: dialog.confirmation.kind });
    },
    cancel: () => {
      if (dialog !== null) setDialog({ ...dialog, isOpen: false });
      mutation.reset();
    },
    isPending: mutation.isPending,
    error: isOpen && mutation.isError ? failure(mutation.error) : undefined,
    status,
  };
}
