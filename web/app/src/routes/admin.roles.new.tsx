// SPDX-License-Identifier: AGPL-3.0-only

// A new role, at /admin/roles/new: its key, its place and its permissions, under the entry Roles. Leaving it with
// changes that are not saved is held back until the reader says whether to. Without a session it asks the reader to
// sign in.

import { loadAdminRoles } from "@my/app-data";
import { NewRoleScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff-server";

export const Route = createFileRoute("/admin/roles/new")({
  loader: ({ context }) => adminSubLoad(context, "manage_roles", loadAdminRoles),
  component: NewRolePage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function NewRolePage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="roles" signedIn={data !== null} returnTo="/admin/roles/new">
      {(sub) => <NewRoleScreen {...sub} />}
    </AdminSubPage>
  );
}
