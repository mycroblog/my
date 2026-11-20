// SPDX-License-Identifier: AGPL-3.0-only

// The roles (FR-720, ADR-0070 §8), on the fake server (app.samples.tsx; staff-roles.ts): the administrator (hana),
// whose highest role is `admin`, and a community lead below the administrators (hana given the role `lead`, place
// 500, which holds `manage_roles` and some of the moderators' and the administrators' permissions), to whom FR-720
// offers less. The list of roles, for both: `admin` above the lead's reach, muted. A role's page: one the reader may
// reshape (`artist`, with a badge), one holding permissions the lead lacks (`moderator`: a sentence names them), the
// built-in `admin` for the administrator (its badge alone) and for the lead (what it is, nothing to change), one with
// no permission that nobody holds (`helper`, with Delete last); loading, failing, and an address that names none. A
// new role's form, for the lead: the permissions they hold alone, and a place no higher than theirs. The roles of an
// account on its Management page: mio's, for the administrator and for the lead.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const meta = {
  title: "Administration/Roles",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/admin/roles", viewer: "hana" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `at` as the administrator reads it. */
const asAdmin = (path: string, more: Omit<Fake, "at"> = {}) => screen({ at: path, viewer: "hana", ...more });

/** The screen at `at` as a community lead reads it: hana holding only the role `lead`, below the administrators. */
const asLead = (path: string) =>
  screen({ at: path, viewer: "hana", server: { staffServer: { roles: { viewer: ["lead"] } } } });

/** Every role, highest first, for the administrator: each its place, its permissions and its badge. */
export const Roles: Story = { parameters: asAdmin("/admin/roles") };
/** Every role for a community lead: `admin` is above their reach, muted, and says so. */
export const RolesForALead: Story = { parameters: asLead("/admin/roles") };
export const RolesLoading: Story = { parameters: asAdmin("/admin/roles", { state: "loading" }) };
export const RolesError: Story = { parameters: asAdmin("/admin/roles", { state: "error" }) };
/** The roles' address, for a moderator, whose permissions do not open it. */
export const RolesNotOpen: Story = { parameters: screen({ at: "/admin/roles", viewer: "marco" }) };

/** A role the administrator may reshape: its place, every permission, its badge, and Delete last. */
export const Role: Story = { parameters: asAdmin("/admin/roles/artist") };
/**
 * The moderators' role as the lead opens it: a place up to theirs, the permissions they hold, and the ones they lack
 * that the role grants, named in a sentence; no Delete, since it is built in.
 */
export const RoleWithPermissionsTheReaderLacks: Story = { parameters: asLead("/admin/roles/moderator") };
/** The built-in `admin` for the administrator: what it is, and its badge, which alone can change. */
export const RoleAdmin: Story = { parameters: asAdmin("/admin/roles/admin") };
/** The built-in `admin` for the lead: what it is, above their reach, nothing to change. */
export const RoleAboveTheReader: Story = { parameters: asLead("/admin/roles/admin") };
/** A role with no permission that nobody holds, which the lead may delete. */
export const RoleNobodyHolds: Story = { parameters: asLead("/admin/roles/helper") };
export const RoleLoading: Story = { parameters: asAdmin("/admin/roles/artist", { state: "loading" }) };
export const RoleError: Story = { parameters: asAdmin("/admin/roles/artist", { state: "error" }) };
/** An address that names no role. */
export const RoleMissing: Story = { parameters: asAdmin("/admin/roles/none") };

/** A new role for the administrator: every permission offered. */
export const NewRole: Story = { parameters: asAdmin("/admin/roles/new") };
/** A new role for the lead: only the permissions they hold, a place no higher than theirs. */
export const NewRoleForALead: Story = { parameters: asLead("/admin/roles/new") };

/**
 * The Management page of mio's account for the administrator: the roles section, `artist` with Revoke, and Grant, a
 * choice of every role mio lacks.
 */
export const AccountRolesForTheAdmin: Story = { parameters: asAdmin("/moderation/accounts/{account:mio}/manage") };
/** The same for the lead: Grant offers only the roles at or below theirs whose permissions they hold. */
export const AccountRolesForALead: Story = { parameters: asLead("/moderation/accounts/{account:mio}/manage") };
