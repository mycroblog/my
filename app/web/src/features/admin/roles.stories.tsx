// SPDX-License-Identifier: AGPL-3.0-only
// The roles (/admin/roles; web-client/screens/admin-roles.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import {
  adminAnswering,
  adminFailingOn,
  adminLoadingOn,
  adminScreen,
  everyPermission,
  withoutPermission,
} from "./test/stories";

/** The built-in roles of every server: admin with every permission, and moderator. */
const adminRoleFixture: Schemas["AdminRole"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000a01",
  key: "admin",
  position: 1000,
  permissions: everyPermission,
};
const moderatorRoleFixture: Schemas["AdminRole"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000a02",
  key: "moderator",
  position: 100,
  permissions: ["manage_reports", "manage_accounts", "view_audit"],
};

/** GET /admin/roles answering `roles`, highest first. */
function rolesOf(roles: Schemas["AdminRole"][]) {
  return http.get(`${apiBase}/admin/roles`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: roles } satisfies Schemas["AdminRoleList"])),
  );
}

/** The built-in roles, badged, and custom ones: a finance team, an emoji team, supporters, and one without a badge. */
const manyRoles = rolesOf([
  { ...adminRoleFixture, badge: { label: "Admin", color: "red" } },
  {
    id: "0192f0c4-7a8e-7000-8000-000000000a05",
    key: "finance",
    position: 500,
    permissions: ["manage_finance", "view_finance", "manage_supporters"],
    badge: { label: "Treasurer", color: "yellow" },
  },
  { ...moderatorRoleFixture, badge: { label: "Moderator", color: "blue" } },
  {
    id: "0192f0c4-7a8e-7000-8000-000000000a06",
    key: "emoji_curators_of_the_birdwatching_club",
    position: 50,
    permissions: ["manage_custom_emojis", "create_custom_emojis"],
    badge: { label: "Emoji curator of the club", color: "teal" },
  },
  {
    id: "0192f0c4-7a8e-7000-8000-000000000a07",
    key: "supporter",
    position: 10,
    permissions: ["create_custom_emojis", "start_voice_rooms", "create_invites"],
    badge: { label: "Supporter", color: "green" },
  },
  {
    id: "0192f0c4-7a8e-7000-8000-000000000a08",
    key: "voice_host",
    position: 5,
    permissions: ["start_voice_rooms"],
  },
]);

const meta = { title: "Screens/Admin/Roles", ...adminScreen("/admin/roles", [manyRoles]) } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The built-in roles and four custom ones, most with a badge of their color, one with a long key and badge. */
export const Normal: Story = {};

/** A new server with only its built-in roles, admin and moderator (the default handlers). */
export const BuiltInOnly: Story = adminAnswering();

/** A moderator without manage_roles. */
export const MissingPermission: Story = withoutPermission;

/** The roles have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/roles");

/** The server failed to answer the roles. */
export const ServerError: Story = adminFailingOn("get", "/admin/roles");

/** In Japanese, with badges labeled in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...adminAnswering(
    rolesOf([
      { ...adminRoleFixture, badge: { label: "管理者", color: "red" } },
      { ...moderatorRoleFixture, badge: { label: "モデレーター", color: "blue" } },
      {
        id: "0192f0c4-7a8e-7000-8000-000000000a07",
        key: "supporter",
        position: 10,
        permissions: ["create_custom_emojis"],
        badge: { label: "いつも応援してくれる人", color: "green" },
      },
    ]),
  ),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the long key and the permissions wrap. */
export const Narrow: Story = narrow;
