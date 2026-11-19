// SPDX-License-Identifier: AGPL-3.0-only

// A new login provider, at /admin/login-providers/new: its kind, connection and who may use it, under the entry Login
// providers; once set up, its page. Leaving it with changes that are not saved is held back until the reader says
// whether to. Without a session it asks the reader to sign in.

import { loadLoginProviders, NewLoginProviderScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/login-providers/new")({
  loader: ({ context }) => adminSubLoad(context, "manage_login_providers", loadLoginProviders),
  component: NewLoginProviderPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function NewLoginProviderPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="login-providers" signedIn={data !== null} returnTo="/admin/login-providers/new">
      {(sub) => <NewLoginProviderScreen {...sub} />}
    </AdminSubPage>
  );
}
