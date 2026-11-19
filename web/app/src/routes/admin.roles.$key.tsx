// SPDX-License-Identifier: AGPL-3.0-only

// A role, at /admin/roles/{key}: its place, its permissions and its badge, a form saved as a whole, and Delete, under
// the entry Roles; what it is, where the reader's roles may not change it. Leaving it with changes that are not saved
// is held back until the reader says whether to. Without a session it asks the reader to sign in.

import { loadAdminRoles } from "@my/app-data";
import { RoleScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/roles/$key")({
  loader: ({ context }) => adminSubLoad(context, "manage_roles", loadAdminRoles),
  component: RolePage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function RolePage() {
  const { data } = Route.useRouteContext();
  const { key } = Route.useParams();
  return (
    <AdminSubPage key={key} entry="roles" signedIn={data !== null} returnTo={`/admin/roles/${encodeURIComponent(key)}`}>
      {(sub) => <RoleScreen roleKey={key} {...sub} />}
    </AdminSubPage>
  );
}
