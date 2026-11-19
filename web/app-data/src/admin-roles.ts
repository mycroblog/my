// SPDX-License-Identifier: AGPL-3.0-only
// The roles as a holder of `manage_roles` keeps them (FR-720, ADR-0070 §8): the roles, highest first; creating one,
// changing its place, its permissions and its badge, and deleting it; and granting a role to a local account or
// revoking it. What the reader may do is bounded by their own roles (`RoleRights`): no role above their highest, no
// permission they lack given or taken, no role granted that holds one, and the built-in roles kept as the server keeps
// them (`admin` changes only its badge, neither it nor `moderator` is deleted). A plan's role is managed from its plan
// (ADR-0072 §5); the API says which roles are plans' only to a holder of `manage_supporters`, whose plans name them.
// Every write reads again what it changes: the roles, the accounts' roles, and the reader's own permissions.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { viewerQuery } from "./accounts";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";
import type { Permission } from "./rights";

/** A role as the administrators manage it. */
export type AdminRole = components["schemas"]["AdminRole"];
/** The badge of a role: its label, and the name of its color. */
export type RoleBadge = components["schemas"]["RoleBadge"];
/** The colors a role's badge may take, in the server's order. */
export type BadgeColor = components["schemas"]["BadgeColor"];
/** Why the server refused a change of a role. */
export type RoleRefusal = components["schemas"]["RoleProblemReason"];

/** The badge colors, in the server's order. */
export const badgeColors = [
  "neutral",
  "red",
  "orange",
  "yellow",
  "green",
  "teal",
  "blue",
  "purple",
  "pink",
] as const satisfies readonly BadgeColor[];

/** The permissions, in the server's order (`Permission`), which a role's permissions are listed in. */
export const permissionOrder = [
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
  "manage_service_closure",
  "start_voice_rooms",
  "manage_errors",
  "manage_login_providers",
  "create_invites",
  "keep_posts_longer",
  "create_articles",
  "create_events",
] as const satisfies readonly Permission[];

/** The rules of a role's fields, as the server checks them. */
export const roleLimits = { key: 64, badgeLabel: 32, position: 999, admin: 1000 } as const;

/** A role's key the server takes: 1 to 64 lowercase ASCII letters, digits, `_` and `-`, other than `new`. */
export const validRoleKey = (key: string): boolean => /^[a-z0-9_-]{1,64}$/.test(key) && key !== "new";

/** The built-in roles: `admin` changes only its badge, and neither is deleted. */
export const builtinRoles: ReadonlySet<string> = new Set(["admin", "moderator"]);

/** The roles of the instance, highest first. */
export function adminRolesQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/roles");
}

/** The roles a local account's user holds, highest first. */
export function accountRolesQuery({ queries }: AppData, accountId: string) {
  return queries.queryOptions("get", "/admin/accounts/{id}/roles", { params: { path: { id: accountId } } });
}

/** The supporter plans, which name the roles they grant; read only by a holder of `manage_supporters`. */
function supporterPlansQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/supporter_plans");
}

/**
 * What the reader may do with roles (FR-720): the position of their highest role, the permissions their roles grant,
 * and the roles that supporter plans grant, which their plans manage.
 */
export type RoleRights = {
  /** The position of the reader's highest role; a role above it is not theirs to change, grant or revoke. */
  readonly top: number;
  /** The key of the reader's highest role; none for a reader who holds none. */
  readonly topKey: string | undefined;
  /** The permissions the reader's roles grant; only these are given or taken, and a role with another is not granted. */
  readonly permissions: ReadonlySet<Permission>;
  /** The IDs of the roles a supporter plan grants: none where the reader may not read the plans. */
  readonly planRoles: ReadonlySet<string>;
};

/** Whether `role` is at or below the reader's highest role: theirs to change, grant and revoke, the rest aside. */
export const withinReach = (role: AdminRole, rights: RoleRights): boolean => role.position <= rights.top;

/** Whether the reader may change the place and the permissions of `role` (not its badge alone). */
export const mayReshape = (role: AdminRole, rights: RoleRights): boolean =>
  withinReach(role, rights) && role.key !== "admin" && !rights.planRoles.has(role.id);

/** Whether the reader may delete `role`: not a built-in one, nor a plan's, and within their reach. */
export const mayDelete = (role: AdminRole, rights: RoleRights): boolean =>
  withinReach(role, rights) && !builtinRoles.has(role.key) && !rights.planRoles.has(role.id);

/** The highest position the reader may give a role: below `admin`, and no higher than their own highest role. */
export const highestPlace = (rights: RoleRights): number => Math.min(roleLimits.position, rights.top);

/**
 * Whether the reader may grant `role` to an account: within their reach, not a plan's, and with no permission the
 * reader lacks.
 */
export const mayGrant = (role: AdminRole, rights: RoleRights): boolean =>
  withinReach(role, rights) &&
  !rights.planRoles.has(role.id) &&
  role.permissions.every((p) => rights.permissions.has(p));

/** Whether the reader may revoke `role` from an account: within their reach, and not a plan's. */
export const mayRevoke = (role: AdminRole, rights: RoleRights): boolean =>
  withinReach(role, rights) && !rights.planRoles.has(role.id);

/** The permissions a role holds that the reader may not take from it, in the server's order. */
export const fixedPermissions = (role: AdminRole, rights: RoleRights): readonly Permission[] =>
  permissionOrder.filter((p) => role.permissions.includes(p) && !rights.permissions.has(p));

/** The permissions the reader may give or take: theirs, in the server's order. */
export const offeredPermissions = (rights: RoleRights): readonly Permission[] =>
  permissionOrder.filter((p) => rights.permissions.has(p));

/**
 * The reader's rights over roles (`RoleRights`), once their own roles are read (and the supporter plans, which a holder
 * of `manage_supporters` reads); undefined until then.
 */
export function useRoleRights(): ScreenRead & { readonly rights: RoleRights | undefined } {
  const data = useAppData();
  const viewer = useQuery(viewerQuery(data)).data;
  const own = useQuery({ ...accountRolesQuery(data, viewer?.id ?? ""), enabled: viewer !== undefined });
  const readsPlans = viewer?.permissions.includes("manage_supporters") === true;
  const plans = useQuery({ ...supporterPlansQuery(data), enabled: readsPlans });
  const reads = readsPlans ? [own, plans] : [own];
  const read = readOf(reads);
  if (viewer === undefined || own.data === undefined || (readsPlans && plans.data === undefined))
    return { ...read, status: read.status === "read" ? "loading" : read.status, rights: undefined };
  return {
    ...read,
    rights: {
      top: Math.max(-1, ...own.data.items.map((r) => r.position)),
      topKey: own.data.items.toSorted((a, b) => b.position - a.position)[0]?.key,
      permissions: new Set(viewer.permissions),
      planRoles: new Set(plans.data?.items.map((p) => p.role.id) ?? []),
    },
  };
}

/** Reads the roles and the reader's own roles, for a route's loader. */
export async function loadAdminRoles(data: AppData): Promise<void> {
  const viewer = await data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
  await Promise.all([
    data.queryClient.query(adminRolesQuery(data)).catch(() => undefined),
    viewer === undefined
      ? undefined
      : data.queryClient.query(accountRolesQuery(data, viewer.id)).catch(() => undefined),
    viewer?.permissions.includes("manage_supporters") === true
      ? data.queryClient.query(supporterPlansQuery(data)).catch(() => undefined)
      : undefined,
  ]);
}

/** The roles, highest first, with the reader's rights over them, and where their reads are. */
export function useAdminRoles(): ScreenRead & {
  readonly roles: readonly AdminRole[] | undefined;
  readonly rights: RoleRights | undefined;
} {
  const query = useQuery(adminRolesQuery(useAppData()));
  const rights = useRoleRights();
  const read = readOf([query]);
  return {
    status: read.status === "read" ? rights.status : read.status,
    retry: () => {
      read.retry();
      rights.retry();
    },
    roles: query.data?.items,
    rights: rights.rights,
  };
}

/** The role of `key`, found in the list, with the reader's rights; `missing` once the list holds none of that key. */
export function useAdminRole(key: string): ReturnType<typeof useAdminRoles> & {
  readonly role: AdminRole | undefined;
  readonly missing: boolean;
} {
  const read = useAdminRoles();
  const role = read.roles?.find((r) => r.key === key);
  return { ...read, role, missing: read.roles !== undefined && role === undefined };
}

/** The roles of the account of `accountId`, highest first, and where their read is. */
export function useAccountRoles(accountId: string): ScreenRead & {
  readonly held: readonly AdminRole[] | undefined;
  /** The server has no local account of that ID (404). */
  readonly missing: boolean;
} {
  const query = useQuery(accountRolesQuery(useAppData(), accountId));
  const missing = query.error instanceof ProblemError && query.error.status === 404;
  return { ...readOf([query]), held: query.data?.items, missing };
}

/** Reads the roles of the account of `accountId`, the roles and the reader's rights, for a route's loader. */
export async function loadAccountRoles(data: AppData, accountId: string): Promise<void> {
  await Promise.all([
    loadAdminRoles(data),
    data.queryClient.query(accountRolesQuery(data, accountId)).catch(() => undefined),
  ]);
}

/** Why the server refused a change of a role (`reason` of a 422 `invalid-role`); undefined for any other failure. */
export function roleRefusal(error: unknown): RoleRefusal | undefined {
  if (!(error instanceof ProblemError) || error.status !== 422) return undefined;
  const reason: unknown = error.problem["reason"];
  return typeof reason === "string" ? (reason as RoleRefusal) : undefined;
}

/** Whether `error` is the server's refusal of a role that a supporter plan manages (409). */
export function roleManagedByPlan(error: unknown): boolean {
  return (
    error instanceof ProblemError &&
    (error.problem.type === "/api/v1/problems/role-managed-by-plan" ||
      error.problem.type === "/api/v1/problems/breaks-supporter-roles")
  );
}

/** A new role: its key, its place, and its permissions. */
export type NewRole = components["schemas"]["CreateRoleRequest"];
/** A change of a role; what it leaves out stays. */
export type RoleChange = components["schemas"]["UpdateRoleRequest"];

/** The writes of roles; each rejects with the server's problem (`roleRefusal` says a 422's reason). */
export type AdminRoleWrites = {
  /** Creates a role, and resolves with it. */
  readonly create: (role: NewRole) => Promise<AdminRole>;
  /** Changes the role of `key`, and resolves with it. */
  readonly change: (key: string, change: RoleChange) => Promise<AdminRole>;
  /** Deletes the role of `key`, which nobody may hold. */
  readonly remove: (key: string) => Promise<void>;
  /** Grants the role of `key` to the account of `accountId`, and resolves with the roles it holds then. */
  readonly grant: (accountId: string, key: string) => Promise<readonly AdminRole[]>;
  /** Revokes the role of `key` from the account of `accountId`, and resolves with the roles it holds then. */
  readonly revoke: (accountId: string, key: string) => Promise<readonly AdminRole[]>;
};

/**
 * Reads again what a change of the roles changes: the roles, every account's roles (a role's place and permissions are
 * theirs too), and the reader's own account, whose permissions follow their roles.
 */
async function rolesChanged(data: AppData): Promise<void> {
  void data.queryClient.invalidateQueries({ queryKey: viewerQuery(data).queryKey });
  void data.queryClient.invalidateQueries({
    predicate: ({ queryKey }) => queryKey[1] === "/admin/accounts/{id}/roles",
  });
  await data.queryClient.invalidateQueries({ queryKey: adminRolesQuery(data).queryKey });
}

/** What an account's roles became, put into the cache, and the reader's own account read again if it is theirs. */
async function heldChanged(
  data: AppData,
  accountId: string,
  list: components["schemas"]["AdminRoleList"] | undefined,
): Promise<readonly AdminRole[]> {
  if (list === undefined) throw new Error("app-data: a grant of a role answered without the roles");
  const key = accountRolesQuery(data, accountId).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  data.queryClient.setQueryData(key, list);
  void data.queryClient.invalidateQueries({ queryKey: viewerQuery(data).queryKey });
  // An account's page of the moderators may list its roles too.
  void data.queryClient.invalidateQueries({
    predicate: ({ queryKey }) => queryKey[1] === "/admin/accounts/{id}" || queryKey[1] === "/accounts/{id}",
  });
  return list.items;
}

/** The writes of roles, for `data`. */
export function adminRoleWrites(data: AppData): AdminRoleWrites {
  const role = (key: string) => ({ params: { path: { role: key } } });
  const account = (id: string, key: string) => ({ params: { path: { id, role: key } } });
  const answered = async (answer: AdminRole | undefined): Promise<AdminRole> => {
    if (answer === undefined) throw new Error("app-data: a write of a role answered without the role");
    await rolesChanged(data);
    return answer;
  };
  return {
    create: async (body) => answered((await data.api.POST("/admin/roles", { body })).data),
    change: async (key, body) => answered((await data.api.PATCH("/admin/roles/{role}", { ...role(key), body })).data),
    remove: async (key) => {
      await data.api.DELETE("/admin/roles/{role}", role(key));
      await rolesChanged(data);
    },
    grant: async (accountId, key) =>
      heldChanged(
        data,
        accountId,
        (await data.api.PUT("/admin/accounts/{id}/roles/{role}", account(accountId, key))).data,
      ),
    revoke: async (accountId, key) =>
      heldChanged(
        data,
        accountId,
        (await data.api.DELETE("/admin/accounts/{id}/roles/{role}", account(accountId, key))).data,
      ),
  };
}

/** The writes of roles, for a screen. */
export function useAdminRoleWrites(): AdminRoleWrites {
  return adminRoleWrites(useAppData());
}
