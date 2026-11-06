// SPDX-License-Identifier: AGPL-3.0-only
// One role (/admin/roles/$key; web-client/screens/admin-role.md) and a new one (/admin/roles/new;
// web-client/screens/admin-role-new.md) in the states of ADR-0079 §13
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { adminAnswering, adminFailingOn, adminScreen, everyPermission, withoutPermission } from "./test/stories";

const adminRole: Schemas["AdminRole"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000a01",
  key: "admin",
  position: 1000,
  permissions: everyPermission,
  badge: { label: "Admin", color: "red" },
};
const curators: Schemas["AdminRole"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000a06",
  key: "emoji_curators_of_the_birdwatching_club",
  position: 50,
  permissions: ["manage_custom_emojis", "create_custom_emojis"],
  badge: { label: "Emoji curator of the club", color: "teal" },
};

/** GET /admin/roles answering `roles`, highest first, and the viewer holding admin. */
function rolesOf(roles: Schemas["AdminRole"][]) {
  return [
    http.get(`${apiBase}/admin/roles`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ items: roles } satisfies Schemas["AdminRoleList"])),
    ),
    http.get(`${apiBase}/admin/accounts/:id/roles`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ items: [adminRole] } satisfies Schemas["AdminRoleList"])),
    ),
  ];
}

const meta = {
  title: "Screens/Admin/Role",
  ...adminScreen(`/admin/roles/${curators.key}`, rolesOf([adminRole, curators])),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A custom role with a long key and a badge: its facts, its form and deleting it. */
export const Normal: Story = {};

/** The built-in admin role: only its badge can change, and it cannot be deleted. */
export const Admin: Story = {
  ...adminScreen("/admin/roles/admin", rolesOf([adminRole, curators])),
};

/** The page of a new role. */
export const New: Story = adminScreen("/admin/roles/new", rolesOf([adminRole, curators]));

/** A moderator without manage_roles. */
export const MissingPermission: Story = withoutPermission;

/** The server failed to answer the roles. */
export const ServerError: Story = adminFailingOn("get", "/admin/roles");

/** In Japanese, with the badge labeled in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...adminAnswering(...rolesOf([adminRole, { ...curators, badge: { label: "絵文字の係", color: "teal" } }])),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the long key and the permissions wrap. */
export const Narrow: Story = narrow;
