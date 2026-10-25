// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the roles of users (FR-720, manage_roles): the roles of the instance, the roles that
// a local account's user holds, and granting and revoking one. The server allows a change only of a role at or
// below the viewer's highest one (403) and keeps the last admin (409); it records every change in the audit log.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";
import { viewerQueryOptions } from "../../session";
import { adminAccountQueryOptions } from "./accounts";
import { isAuditLogQuery, isMissingPermission } from "./staff";

type Schemas = components["schemas"];

export type AdminRole = Schemas["AdminRole"];
export type AdminRoleList = Schemas["AdminRoleList"];

/** The role that every instance has, with every permission; the server never takes it from its last holder. */
export const adminRoleKey = "admin";

/** The roles of the instance, highest position first (GET /admin/roles). */
export function rolesQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/roles");
}

/** The roles that the user of local account `id` holds, highest position first (GET /admin/accounts/{id}/roles). */
export function accountRolesQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/admin/accounts/{id}/roles", { params: { path: { id } } });
}

/** Granting or revoking a role. */
export type RoleChangeKind = "grant" | "revoke";

/** One change: of which account, which role (its key), and whether it is granted or revoked. */
export type RoleChange = { accountId: string; role: string; kind: RoleChangeKind };

/**
 * Grants a role to a local account (PUT /admin/accounts/{id}/roles/{role}) or revokes it (DELETE), as the options
 * of TanStack Query's useMutation (web-client/10-api-client-and-auth.md §10). Answers the roles that the user holds afterwards.
 */
export function changeRoleMutation(api: Api) {
  return {
    mutationKey: ["put", "/admin/accounts/{id}/roles/{role}"] as const,
    mutationFn: async ({ accountId, role, kind }: RoleChange): Promise<AdminRoleList> => {
      const init = { params: { path: { id: accountId, role } } };
      const { data, error } =
        kind === "grant" ? await api.fetch.PUT("/admin/accounts/{id}/roles/{role}", init) : await api.fetch.DELETE("/admin/accounts/{id}/roles/{role}", init);
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * What a change of the roles of account `accountId` changes (web-client/README.md S1, ADR-0045): its page shows its role
 * keys (the searches show none) and the audit log records the change: both are marked stale, and this waits for
 * those on screen to be fetched again (the log is fetched again when it opens); then the roles as the server
 * answered go into the cache. A change of the viewer's own
 * roles (`viewerId`) also marks the viewer, whose permissions decide what the app offers.
 */
export async function cacheChangedRoles(api: Api, queryClient: QueryClient, accountId: string, roles: AdminRoleList, viewerId: string): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: adminAccountQueryOptions(api, accountId).queryKey }),
    queryClient.invalidateQueries({ predicate: (query) => isAuditLogQuery(query.queryKey) }),
  ]);
  queryClient.setQueryData(accountRolesQueryOptions(api, accountId).queryKey, roles);
  if (accountId === viewerId) void queryClient.invalidateQueries({ queryKey: viewerQueryOptions(api).queryKey });
}

/** Marks the viewer's own roles, after a refusal that says they changed since they loaded (isRoleOutranked). */
export function refreshViewerRoles(api: Api, queryClient: QueryClient, viewerId: string): void {
  void queryClient.invalidateQueries({ queryKey: accountRolesQueryOptions(api, viewerId).queryKey });
}

/** Whether the server refused a change of a role above every role of the viewer (403, not a missing permission). */
export function isRoleOutranked(error: unknown): boolean {
  return error instanceof ApiError && error.status === 403 && !isMissingPermission(error);
}

/** Whether the server refused to revoke the admin role from its only holder (409). */
export function isLastAdmin(error: unknown): boolean {
  return error instanceof ApiError && error.status === 409 && error.type === "about:blank";
}
