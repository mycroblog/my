// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the roles screen (web-client/README.md S1, FR-720, manage_roles): the roles of the instance
// (GET /admin/roles, shared with an account's roles region), creating one (POST /admin/roles), changing its position,
// permissions and badge (PATCH /admin/roles/{role}, ADR-0071) and deleting it (DELETE). The server keeps the built-in
// roles (admin changes only its badge; neither admin nor moderator is deleted), refuses a role above the viewer's highest one (403) and
// permissions the viewer lacks, and deletes no role that someone holds: a refusal is a 422 `invalid-role` problem
// whose `reason` says which. Every change is recorded in the audit log.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "../../../lib/api/schema.gen";
import { ApiError } from "../../../lib/errors";
import { type Permission, refreshAuditLog, rolesQueryOptions } from "../../moderation";
import { viewerQueryOptions } from "../../session";

type Schemas = components["schemas"];

export type AdminRole = Schemas["AdminRole"];
export type NewRole = Schemas["CreateRoleRequest"];
export type RoleChange = Schemas["UpdateRoleRequest"];
export type RoleProblemReason = Schemas["RoleProblemReason"];

/** The problem type of a role change that breaks a rule. */
const invalidRoleProblem = "/api/v1/problems/invalid-role";

/** The longest name of a badge, in characters (RoleBadge). */
export const badgeLabelLimit = 32;

/** The built-in roles: admin changes only its badge, and neither is deleted. */
export const adminRoleKey = "admin";
export const moderatorRoleKey = "moderator";

/** The longest key, and the highest position of a role other than admin (CreateRoleRequest). */
export const roleKeyLimit = 64;
export const maxRolePosition = 999;

/** What a key is made of (CreateRoleRequest): lowercase ASCII letters, digits, `_` and `-`. */
export const roleKeyPattern = /^[a-z0-9_-]+$/;

/**
 * The permissions by the area they belong to (ADR-0058, ADR-0070, `Permission.Category` of the server),
 * each in the order of the API's `Permission`.
 */
export const permissionCategories: readonly { category: "moderation" | "server" | "member"; permissions: readonly Permission[] }[] = [
  { category: "moderation", permissions: ["manage_reports", "manage_accounts", "manage_domains", "manage_invites", "view_audit"] },
  {
    category: "server",
    permissions: ["manage_rules", "manage_settings", "manage_roles", "manage_jobs", "manage_login_recovery", "manage_custom_emojis", "manage_webhooks", "manage_announcements", "manage_finance", "manage_supporters", "manage_oauth_apps"],
  },
  { category: "member", permissions: ["view_finance", "create_custom_emojis"] },
];

/** Every permission, in the order of the API's `Permission`. */
export const allPermissions: readonly Permission[] = [
  "manage_reports",
  "manage_accounts",
  "manage_domains",
  "manage_rules",
  "manage_settings",
  "manage_roles",
  "manage_invites",
  "view_audit",
  "manage_jobs",
  "manage_login_recovery",
  "manage_custom_emojis",
  "manage_webhooks",
  "manage_announcements",
  "manage_finance",
  "view_finance",
  "manage_supporters",
  "create_custom_emojis",
  "manage_oauth_apps",
];

/** The reason of an `invalid-role` problem and the permissions it names, or undefined for another error. */
export function roleProblem(error: unknown): { reason: RoleProblemReason | undefined; missing: string[] } | undefined {
  if (!(error instanceof ApiError) || error.type !== invalidRoleProblem) return undefined;
  const reason = error.document["reason"];
  const missing = error.document["missing_permissions"];
  return {
    reason: typeof reason === "string" ? (reason as RoleProblemReason) : undefined,
    missing: Array.isArray(missing) ? missing.filter((p): p is string => typeof p === "string") : [],
  };
}

/** Creates a role (POST /admin/roles), as the options of useMutation. */
export function createRoleMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/roles"] as const,
    mutationFn: async (body: NewRole): Promise<AdminRole> => {
      const { data, error } = await api.fetch.POST("/admin/roles", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Changes a role (PATCH /admin/roles/{role}), as the options of useMutation. */
export function updateRoleMutation(api: Api) {
  return {
    mutationKey: ["patch", "/admin/roles/{role}"] as const,
    mutationFn: async ({ key, change }: { key: string; change: RoleChange }): Promise<AdminRole> => {
      const { data, error } = await api.fetch.PATCH("/admin/roles/{role}", { params: { path: { role: key } }, body: change });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Deletes a role (DELETE /admin/roles/{role}), as the options of useMutation. */
export function deleteRoleMutation(api: Api) {
  return {
    mutationKey: ["delete", "/admin/roles/{role}"] as const,
    mutationFn: async (key: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.DELETE("/admin/roles/{role}", { params: { path: { role: key } } });
    },
  };
}

/**
 * Marks what a change of a role changes (ADR-0045): the roles of the instance, the roles of every account (which
 * carry their permissions), the viewer (whose permissions a change of a role they hold changes), the profiles (whose
 * badges a change of a badge changes), the viewer's badge choices and the audit log; the queries shown now are fetched
 * again and awaited.
 */
export async function refreshRoles(api: Api, queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: rolesQueryOptions(api).queryKey }),
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/accounts/{id}/roles"] }),
    queryClient.invalidateQueries({ queryKey: viewerQueryOptions(api).queryKey }),
    queryClient.invalidateQueries({ queryKey: ["get", "/accounts/lookup"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/preferences/badges"] }),
    refreshAuditLog(queryClient),
  ]);
}
