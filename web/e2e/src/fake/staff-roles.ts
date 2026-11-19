// SPDX-License-Identifier: AGPL-3.0-only

// The fake's roles (FR-720, ADR-0070 §8): the roles of the instance as a holder of `manage_roles` keeps them under
// /admin/roles, and the roles of the scenario's people under /admin/accounts/{id}/roles, checked as the server checks
// them: no role above the reader's highest, no permission the reader lacks given, taken, or granted with a role, the
// built-in roles kept (`admin` changes only its badge, neither it nor `moderator` is deleted), a held role not deleted,
// the last administrator kept. The roles: the built-in `admin` and `moderator`; `lead`, a community lead below the
// administrators with a badge, which the stories give the reader to show what a lower holder of `manage_roles` is
// offered; `artist`, which lets mio request custom emoji; `helper`, with no permission and nobody holding it.

import type { StaffServerContext } from "./staff-server";
import type { Answer, Handlers, Schema } from "./spec";

type Role = Schema<"AdminRole">;
type Permission = Schema<"Permission">;

/** Every permission, in the server's order: the built-in `admin` role's. */
const everyPermission: readonly Permission[] = [
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
];

/** The roles the fake starts with, highest first; their IDs are made from the fake's start. */
const startingRoles: readonly Omit<Role, "id">[] = [
  { key: "admin", position: 1000, permissions: [...everyPermission] },
  {
    key: "lead",
    position: 500,
    permissions: [
      "manage_reports",
      "manage_accounts",
      "manage_roles",
      "view_audit",
      "manage_custom_emojis",
      "manage_announcements",
      "create_custom_emojis",
    ],
    badge: { label: "Community lead", color: "teal" },
  },
  { key: "moderator", position: 100, permissions: ["manage_reports", "manage_accounts", "view_audit"] },
  {
    key: "artist",
    position: 20,
    permissions: ["create_custom_emojis"],
    badge: { label: "Emoji artist", color: "purple" },
  },
  { key: "helper", position: 10, permissions: [] },
];

/** The roles the scenario's people hold at first, by username: hana administers, marco moderates, mio draws emoji. */
const startingHeld: Readonly<Record<string, readonly string[]>> = {
  hana: ["admin"],
  marco: ["moderator"],
  mio: ["artist"],
};

/** The permissions the roles of `keys` grant together, in the server's order: what a reader holding them may do. */
export function permissionsOfRoles(keys: readonly string[]): Permission[] {
  const granted = new Set(startingRoles.filter((r) => keys.includes(r.key)).flatMap((r) => r.permissions));
  return everyPermission.filter((p) => granted.has(p));
}

/** What the stories ask of the fake's roles. */
export type FakeRoleOptions = {
  /**
   * The roles the reader holds in place of the scenario's, their permissions the reader's (`["lead"]`: a holder of
   * `manage_roles` below the administrators).
   */
  readonly viewer?: readonly string[];
};

/** What the roles' handlers take from the fake. */
export type RoleContext = StaffServerContext;

export function roleHandlers(c: RoleContext): { handlers: Handlers } {
  const roles: Role[] = startingRoles.map((r, i) => ({ ...structuredClone(r), id: idOf(c, i) }));
  const held = new Map<string, Set<string>>(
    Object.entries(startingHeld).map(([username, keys]) => [username, new Set(keys)]),
  );
  const viewerRoles = c.options?.roles?.viewer;
  if (viewerRoles !== undefined) held.set(c.viewer, new Set(viewerRoles));

  const sorted = () => roles.toSorted((a, b) => b.position - a.position);
  const rolesOf = (username: string) => sorted().filter((r) => held.get(username)?.has(r.key) === true);
  const top = () => Math.max(-1, ...rolesOf(c.viewer).map((r) => r.position));
  const mine = () => new Set(c.permissions());
  const allowed = () => !c.visitor && c.permissions().includes("manage_roles");
  const forbidden = () => c.problem(403, "Forbidden");
  const refused = (reason: Schema<"RoleProblemReason">, missing?: readonly Permission[]) =>
    c.problem(422, "The role breaks a rule", {
      type: "/api/v1/problems/invalid-role",
      reason,
      ...(missing === undefined ? {} : { missing_permissions: [...missing] }),
    });
  const list = (items: readonly Role[]) => ({ status: 200 as const, body: { items: structuredClone([...items]) } });
  const usernameOfAccount = (id: string) => {
    const account = c.accountById(id);
    return account === undefined || account.acct.includes("@") ? undefined : account.username;
  };
  /** The permissions of a change the reader lacks: those it adds or takes away that their roles do not grant. */
  const lacking = (before: readonly Permission[], after: readonly Permission[]) =>
    everyPermission.filter((p) => before.includes(p) !== after.includes(p) && !mine().has(p));

  const handlers: Handlers = {
    "GET /admin/roles": () => (allowed() ? list(sorted()) : forbidden()),
    "POST /admin/roles": ({ body }) => {
      if (!allowed()) return forbidden();
      if (body.position > top()) return forbidden();
      if (!/^[a-z0-9_-]{1,64}$/.test(body.key) || body.key === "new") return refused("key");
      if (roles.some((r) => r.key === body.key)) return refused("key_taken");
      if (body.position < 0 || body.position > 999) return refused("position");
      if (new Set(body.permissions).size !== body.permissions.length) return refused("permissions");
      const missing = lacking([], body.permissions);
      if (missing.length > 0) return refused("missing_permission", missing);
      const role: Role = {
        id: c.idAt(Date.now(), 71_000 + roles.length),
        key: body.key,
        position: body.position,
        permissions: everyPermission.filter((p) => body.permissions.includes(p)),
      };
      roles.push(role);
      return { status: 201, body: structuredClone(role) };
    },
    "PATCH /admin/roles/{role}": ({ params, body }): Answer<"PATCH /admin/roles/{role}"> => {
      if (!allowed()) return forbidden();
      const role = roles.find((r) => r.key === params.role);
      if (role === undefined) return c.notFound();
      if (role.position > top() || (body.position !== undefined && body.position > top())) return forbidden();
      if (role.key === "admin" && (body.position !== undefined || body.permissions !== undefined))
        return refused("builtin");
      if (body.position !== undefined && (body.position < 0 || body.position > 999)) return refused("position");
      if (body.permissions !== undefined) {
        const missing = lacking(role.permissions, body.permissions);
        if (missing.length > 0) return refused("missing_permission", missing);
      }
      if (body.badge !== undefined && body.remove_badge === true)
        return c.problem(422, "Unprocessable Content", { detail: "Send a badge or remove it.", field: "remove_badge" });
      if (body.badge !== undefined) {
        const label = body.badge.label.trim();
        if (label === "" || Array.from(label).length > 32 || label.includes("\n")) return refused("badge_label");
        role.badge = { label, color: body.badge.color };
      }
      if (body.remove_badge === true) delete role.badge;
      if (body.position !== undefined) role.position = body.position;
      if (body.permissions !== undefined)
        role.permissions = everyPermission.filter((p) => body.permissions?.includes(p) === true);
      return { status: 200, body: structuredClone(role) };
    },
    "DELETE /admin/roles/{role}": ({ params }) => {
      if (!allowed()) return forbidden();
      const role = roles.find((r) => r.key === params.role);
      if (role === undefined) return c.notFound();
      if (role.position > top()) return forbidden();
      if (role.key === "admin" || role.key === "moderator") return refused("builtin");
      if ([...held.values()].some((keys) => keys.has(role.key))) return refused("held");
      roles.splice(roles.indexOf(role), 1);
      return { status: 204 };
    },
    "GET /admin/accounts/{id}/roles": ({ params }) => {
      if (!allowed()) return forbidden();
      const username = usernameOfAccount(params.id);
      return username === undefined ? c.notFound() : list(rolesOf(username));
    },
    "PUT /admin/accounts/{id}/roles/{role}": ({ params }) => {
      if (!allowed()) return forbidden();
      const username = usernameOfAccount(params.id);
      const role = roles.find((r) => r.key === params.role);
      if (username === undefined || role === undefined) return c.notFound();
      if (role.position > top() || role.permissions.some((p) => !mine().has(p))) return forbidden();
      const keys = held.get(username) ?? new Set<string>();
      keys.add(role.key);
      held.set(username, keys);
      return list(rolesOf(username));
    },
    "DELETE /admin/accounts/{id}/roles/{role}": ({ params }) => {
      if (!allowed()) return forbidden();
      const username = usernameOfAccount(params.id);
      const role = roles.find((r) => r.key === params.role);
      if (username === undefined || role === undefined) return c.notFound();
      if (role.position > top()) return forbidden();
      const admins = [...held.values()].filter((keys) => keys.has("admin")).length;
      if (role.key === "admin" && admins <= 1 && held.get(username)?.has("admin") === true)
        return c.problem(409, "Conflict");
      held.get(username)?.delete(role.key);
      return list(rolesOf(username));
    },
  };
  return { handlers };
}

/** The ID of the fake's starting role at `index`. */
const idOf = (c: RoleContext, index: number) => c.idAt(c.started - 900 * 24 * 3_600_000, 70_000 + index);
